package catalog

import (
	"fmt"
	"net/http"
	"strings"

	"fieldserve/internal/audit"
	"fieldserve/internal/auth"
	"fieldserve/internal/platform/httpx"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// HandleAdminListCategories returns all categories ordered by sort_order then name.
func (s *Service) HandleAdminListCategories(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, parent_id, name, slug, sort_order, created_at::text FROM categories ORDER BY sort_order, name`)
	if err != nil {
		return fmt.Errorf("catalog: list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]Category, 0)
	for rows.Next() {
		var cat Category
		if err := rows.Scan(&cat.ID, &cat.ParentID, &cat.Name, &cat.Slug, &cat.SortOrder, &cat.CreatedAt); err != nil {
			return fmt.Errorf("catalog: scan category: %w", err)
		}
		categories = append(categories, cat)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate categories: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"categories": categories})
}

// HandleAdminCreateCategory creates a new category.
func (s *Service) HandleAdminCreateCategory(c echo.Context) error {
	var req struct {
		Name      string  `json:"name"`
		Slug      string  `json:"slug"`
		ParentID  *string `json:"parent_id"`
		SortOrder int     `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	fieldErrors := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		fieldErrors["name"] = []string{"Name is required."}
	}
	if strings.TrimSpace(req.Slug) == "" {
		fieldErrors["slug"] = []string{"Slug is required."}
	}
	if len(fieldErrors) > 0 {
		return httpx.NewValidationError(fieldErrors)
	}

	var cat Category
	err := s.db.QueryRowContext(c.Request().Context(),
		`INSERT INTO categories (name, slug, parent_id, sort_order) VALUES ($1, $2, $3, $4) RETURNING id, parent_id, name, slug, sort_order, created_at::text`,
		req.Name, req.Slug, req.ParentID, req.SortOrder,
	).Scan(&cat.ID, &cat.ParentID, &cat.Name, &cat.Slug, &cat.SortOrder, &cat.CreatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return httpx.NewConflictError("duplicate_slug", "A category with this slug already exists.")
		}
		return fmt.Errorf("catalog: create category: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "category_created",
		ActorID:      user.ID,
		ResourceType: "category",
		ResourceID:   cat.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusCreated, map[string]interface{}{"category": cat})
}

// HandleAdminUpdateCategory partially updates a category.
func (s *Service) HandleAdminUpdateCategory(c echo.Context) error {
	categoryID := c.Param("categoryId")

	var req struct {
		Name      *string `json:"name"`
		Slug      *string `json:"slug"`
		ParentID  *string `json:"parent_id"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Slug != nil {
		setClauses = append(setClauses, fmt.Sprintf("slug = $%d", argIdx))
		args = append(args, *req.Slug)
		argIdx++
	}
	if req.ParentID != nil {
		setClauses = append(setClauses, fmt.Sprintf("parent_id = $%d", argIdx))
		args = append(args, *req.ParentID)
		argIdx++
	}
	if req.SortOrder != nil {
		setClauses = append(setClauses, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}

	if len(setClauses) == 0 {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "No fields to update.")
	}

	query := fmt.Sprintf(
		`UPDATE categories SET %s WHERE id = $%d RETURNING id, parent_id, name, slug, sort_order, created_at::text`,
		strings.Join(setClauses, ", "), argIdx,
	)
	args = append(args, categoryID)

	var cat Category
	err := s.db.QueryRowContext(c.Request().Context(), query, args...).
		Scan(&cat.ID, &cat.ParentID, &cat.Name, &cat.Slug, &cat.SortOrder, &cat.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return httpx.NewNotFoundError("Category not found.")
		}
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return httpx.NewConflictError("duplicate_slug", "A category with this slug already exists.")
		}
		return fmt.Errorf("catalog: update category: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "category_updated",
		ActorID:      user.ID,
		ResourceType: "category",
		ResourceID:   cat.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusOK, map[string]interface{}{"category": cat})
}

// HandleAdminListTags returns all tags ordered by name.
func (s *Service) HandleAdminListTags(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, name, created_at::text FROM tags ORDER BY name`)
	if err != nil {
		return fmt.Errorf("catalog: list tags: %w", err)
	}
	defer rows.Close()

	tags := make([]Tag, 0)
	for rows.Next() {
		var tag Tag
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.CreatedAt); err != nil {
			return fmt.Errorf("catalog: scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate tags: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"tags": tags})
}

// HandleAdminCreateTag creates a new tag.
func (s *Service) HandleAdminCreateTag(c echo.Context) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	if strings.TrimSpace(req.Name) == "" {
		return httpx.NewValidationError(map[string][]string{"name": {"Name is required."}})
	}

	var tag Tag
	err := s.db.QueryRowContext(c.Request().Context(),
		`INSERT INTO tags (name) VALUES ($1) RETURNING id, name, created_at::text`,
		req.Name,
	).Scan(&tag.ID, &tag.Name, &tag.CreatedAt)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return httpx.NewConflictError("duplicate_tag", "A tag with this name already exists.")
		}
		return fmt.Errorf("catalog: create tag: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "tag_created",
		ActorID:      user.ID,
		ResourceType: "tag",
		ResourceID:   tag.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusCreated, map[string]interface{}{"tag": tag})
}

// HandleAdminUpdateTag updates a tag's name.
func (s *Service) HandleAdminUpdateTag(c echo.Context) error {
	tagID := c.Param("tagId")

	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	if strings.TrimSpace(req.Name) == "" {
		return httpx.NewValidationError(map[string][]string{"name": {"Name is required."}})
	}

	var tag Tag
	err := s.db.QueryRowContext(c.Request().Context(),
		`UPDATE tags SET name = $1 WHERE id = $2 RETURNING id, name, created_at::text`,
		req.Name, tagID,
	).Scan(&tag.ID, &tag.Name, &tag.CreatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return httpx.NewNotFoundError("Tag not found.")
		}
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return httpx.NewConflictError("duplicate_tag", "A tag with this name already exists.")
		}
		return fmt.Errorf("catalog: update tag: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "tag_updated",
		ActorID:      user.ID,
		ResourceType: "tag",
		ResourceID:   tag.ID,
	})

	s.notifyCatalogChange()
	return c.JSON(http.StatusOK, map[string]interface{}{"tag": tag})
}

// ---------- Search config admin handlers ----------

// HotKeyword represents a row in search_keyword_config.
type HotKeyword struct {
	ID        string `json:"id"`
	Keyword   string `json:"keyword"`
	IsHot     bool   `json:"is_hot"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AutocompleteTerm represents a row in autocomplete_terms.
type AutocompleteTerm struct {
	ID        string `json:"id"`
	Term      string `json:"term"`
	Weight    int    `json:"weight"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// HandleAdminListHotKeywords lists all hot-keyword config entries.
func (s *Service) HandleAdminListHotKeywords(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, keyword, is_hot, created_at::text, updated_at::text FROM search_keyword_config ORDER BY keyword`)
	if err != nil {
		return fmt.Errorf("catalog: list hot keywords: %w", err)
	}
	defer rows.Close()

	keywords := make([]HotKeyword, 0)
	for rows.Next() {
		var kw HotKeyword
		if err := rows.Scan(&kw.ID, &kw.Keyword, &kw.IsHot, &kw.CreatedAt, &kw.UpdatedAt); err != nil {
			return fmt.Errorf("catalog: scan hot keyword: %w", err)
		}
		keywords = append(keywords, kw)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate hot keywords: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"keywords": keywords})
}

// HandleAdminCreateHotKeyword creates a new hot-keyword config entry.
func (s *Service) HandleAdminCreateHotKeyword(c echo.Context) error {
	var req struct {
		Keyword string `json:"keyword"`
		IsHot   *bool  `json:"is_hot"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	if strings.TrimSpace(req.Keyword) == "" {
		return httpx.NewValidationError(map[string][]string{"keyword": {"Keyword is required."}})
	}

	isHot := false
	if req.IsHot != nil {
		isHot = *req.IsHot
	}

	var kw HotKeyword
	err := s.db.QueryRowContext(c.Request().Context(),
		`INSERT INTO search_keyword_config (keyword, is_hot) VALUES ($1, $2)
		 RETURNING id, keyword, is_hot, created_at::text, updated_at::text`,
		req.Keyword, isHot,
	).Scan(&kw.ID, &kw.Keyword, &kw.IsHot, &kw.CreatedAt, &kw.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: create hot keyword: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "hot_keyword_created",
		ActorID:      user.ID,
		ResourceType: "search_keyword_config",
		ResourceID:   kw.ID,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"keyword": kw})
}

// HandleAdminUpdateHotKeyword updates a hot-keyword config entry.
func (s *Service) HandleAdminUpdateHotKeyword(c echo.Context) error {
	keywordID := c.Param("keywordId")

	var req struct {
		Keyword *string `json:"keyword"`
		IsHot   *bool   `json:"is_hot"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Keyword != nil {
		setClauses = append(setClauses, fmt.Sprintf("keyword = $%d", argIdx))
		args = append(args, *req.Keyword)
		argIdx++
	}
	if req.IsHot != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_hot = $%d", argIdx))
		args = append(args, *req.IsHot)
		argIdx++
	}

	if len(setClauses) == 0 {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "No fields to update.")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(
		`UPDATE search_keyword_config SET %s WHERE id = $%d
		 RETURNING id, keyword, is_hot, created_at::text, updated_at::text`,
		strings.Join(setClauses, ", "), argIdx,
	)
	args = append(args, keywordID)

	var kw HotKeyword
	err := s.db.QueryRowContext(c.Request().Context(), query, args...).
		Scan(&kw.ID, &kw.Keyword, &kw.IsHot, &kw.CreatedAt, &kw.UpdatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return httpx.NewNotFoundError("Hot keyword not found.")
		}
		return fmt.Errorf("catalog: update hot keyword: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "hot_keyword_updated",
		ActorID:      user.ID,
		ResourceType: "search_keyword_config",
		ResourceID:   kw.ID,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"keyword": kw})
}

// HandleAdminListAutocomplete lists all autocomplete terms.
func (s *Service) HandleAdminListAutocomplete(c echo.Context) error {
	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, term, weight, created_at::text, updated_at::text FROM autocomplete_terms ORDER BY weight DESC, term`)
	if err != nil {
		return fmt.Errorf("catalog: list autocomplete: %w", err)
	}
	defer rows.Close()

	terms := make([]AutocompleteTerm, 0)
	for rows.Next() {
		var t AutocompleteTerm
		if err := rows.Scan(&t.ID, &t.Term, &t.Weight, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return fmt.Errorf("catalog: scan autocomplete: %w", err)
		}
		terms = append(terms, t)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("catalog: iterate autocomplete: %w", err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"terms": terms})
}

// HandleAdminCreateAutocomplete creates a new autocomplete term.
func (s *Service) HandleAdminCreateAutocomplete(c echo.Context) error {
	var req struct {
		Term   string `json:"term"`
		Weight int    `json:"weight"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	if strings.TrimSpace(req.Term) == "" {
		return httpx.NewValidationError(map[string][]string{"term": {"Term is required."}})
	}

	var t AutocompleteTerm
	err := s.db.QueryRowContext(c.Request().Context(),
		`INSERT INTO autocomplete_terms (term, weight) VALUES ($1, $2)
		 RETURNING id, term, weight, created_at::text, updated_at::text`,
		req.Term, req.Weight,
	).Scan(&t.ID, &t.Term, &t.Weight, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("catalog: create autocomplete: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "autocomplete_created",
		ActorID:      user.ID,
		ResourceType: "autocomplete_terms",
		ResourceID:   t.ID,
	})

	return c.JSON(http.StatusCreated, map[string]interface{}{"term": t})
}

// HandleAdminUpdateAutocomplete updates an autocomplete term.
func (s *Service) HandleAdminUpdateAutocomplete(c echo.Context) error {
	termID := c.Param("termId")

	var req struct {
		Term   *string `json:"term"`
		Weight *int    `json:"weight"`
	}
	if err := c.Bind(&req); err != nil {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "Invalid request body.")
	}

	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Term != nil {
		setClauses = append(setClauses, fmt.Sprintf("term = $%d", argIdx))
		args = append(args, *req.Term)
		argIdx++
	}
	if req.Weight != nil {
		setClauses = append(setClauses, fmt.Sprintf("weight = $%d", argIdx))
		args = append(args, *req.Weight)
		argIdx++
	}

	if len(setClauses) == 0 {
		return httpx.NewAPIError(http.StatusBadRequest, "bad_request", "No fields to update.")
	}

	setClauses = append(setClauses, "updated_at = NOW()")

	query := fmt.Sprintf(
		`UPDATE autocomplete_terms SET %s WHERE id = $%d
		 RETURNING id, term, weight, created_at::text, updated_at::text`,
		strings.Join(setClauses, ", "), argIdx,
	)
	args = append(args, termID)

	var t AutocompleteTerm
	err := s.db.QueryRowContext(c.Request().Context(), query, args...).
		Scan(&t.ID, &t.Term, &t.Weight, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return httpx.NewNotFoundError("Autocomplete term not found.")
		}
		return fmt.Errorf("catalog: update autocomplete: %w", err)
	}

	user := auth.UserFromContext(c)
	s.audit.Log(c.Request().Context(), audit.Event{
		EventType:    "autocomplete_updated",
		ActorID:      user.ID,
		ResourceType: "autocomplete_terms",
		ResourceID:   t.ID,
	})

	return c.JSON(http.StatusOK, map[string]interface{}{"term": t})
}
