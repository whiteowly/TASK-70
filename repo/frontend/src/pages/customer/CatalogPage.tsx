import { useState, useEffect, useCallback, useRef } from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { catalogApi, customerApi } from "../../api/catalog";
import type { ServiceSummary } from "../../api/catalog";
import { useCompareStore, MAX_COMPARE } from "../../stores/compare";

const SORT_OPTIONS = [
  { value: "relevance", label: "Relevance" },
  { value: "newest", label: "Newest" },
  { value: "price_asc", label: "Price (Low)" },
  { value: "price_desc", label: "Price (High)" },
  { value: "popularity", label: "Most Popular" },
  { value: "rating", label: "Highest Rated" },
  { value: "distance", label: "Distance" },
];

function formatPrice(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

function CatalogPage() {
  const pageSize = 20;

  // Search & filter state
  const [searchInput, setSearchInput] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [selectedTagIds, setSelectedTagIds] = useState<string[]>([]);
  const [minPrice, setMinPrice] = useState("");
  const [maxPrice, setMaxPrice] = useState("");
  const [minRating, setMinRating] = useState("");
  const [radiusMiles, setRadiusMiles] = useState("");
  const [availableDate, setAvailableDate] = useState("");
  const [availableTime, setAvailableTime] = useState("");
  const [sort, setSort] = useState("newest");
  const [page, setPage] = useState(1);
  const [showFilters, setShowFilters] = useState(false);

  // Autocomplete state
  const [autocompleteTerm, setAutocompleteTerm] = useState("");
  const [showAutocomplete, setShowAutocomplete] = useState(false);
  const [selectedSuggestionIdx, setSelectedSuggestionIdx] = useState(-1);
  const autocompleteRef = useRef<HTMLDivElement>(null);

  // Favorites state
  const [favoriteIds, setFavoriteIds] = useState<Set<string>>(new Set());

  // Compare store
  const compareAdd = useCompareStore((s) => s.add);
  const compareRemove = useCompareStore((s) => s.remove);
  const compareHas = useCompareStore((s) => s.has);
  useCompareStore((s) => s.items);

  // Compare overflow feedback
  const [compareAlert, setCompareAlert] = useState<string | null>(null);

  // Debounce search input for main search
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQ(searchInput);
      setPage(1);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Debounce autocomplete
  useEffect(() => {
    if (!searchInput) {
      setAutocompleteTerm("");
      setShowAutocomplete(false);
      return;
    }
    const timer = setTimeout(() => {
      setAutocompleteTerm(searchInput);
    }, 200);
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Close autocomplete on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (
        autocompleteRef.current &&
        !autocompleteRef.current.contains(e.target as Node)
      ) {
        setShowAutocomplete(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  // Load categories & tags for filters
  const { data: categoriesData } = useQuery({
    queryKey: ["catalog-categories"],
    queryFn: catalogApi.listCategories,
  });
  const { data: tagsData } = useQuery({
    queryKey: ["catalog-tags"],
    queryFn: catalogApi.listTags,
  });

  // Load favorites
  const { data: favoritesData } = useQuery({
    queryKey: ["customer-favorites"],
    queryFn: customerApi.getFavorites,
  });

  useEffect(() => {
    if (favoritesData?.favorites) {
      setFavoriteIds(
        new Set(favoritesData.favorites.map((f) => f.service_id)),
      );
    }
  }, [favoritesData]);

  // Search history
  const { data: historyData } = useQuery({
    queryKey: ["customer-search-history"],
    queryFn: customerApi.getSearchHistory,
  });

  // Hot keywords
  const { data: hotKeywordsData } = useQuery({
    queryKey: ["catalog-hot-keywords"],
    queryFn: catalogApi.getHotKeywords,
  });

  // Autocomplete suggestions
  const { data: autocompleteData } = useQuery({
    queryKey: ["catalog-autocomplete", autocompleteTerm],
    queryFn: () => catalogApi.getAutocomplete(autocompleteTerm),
    enabled: autocompleteTerm.length >= 2,
  });

  // Show autocomplete when we have suggestions
  useEffect(() => {
    if (
      autocompleteData?.terms &&
      autocompleteData.terms.length > 0 &&
      autocompleteTerm.length >= 2
    ) {
      setShowAutocomplete(true);
      setSelectedSuggestionIdx(-1);
    } else {
      setShowAutocomplete(false);
    }
  }, [autocompleteData, autocompleteTerm]);

  // Trending (shown when no search query)
  const { data: trendingData } = useQuery({
    queryKey: ["catalog-trending"],
    queryFn: catalogApi.getTrending,
    enabled: !debouncedQ,
  });

  // Main search query
  const { data, isLoading, error } = useQuery({
    queryKey: [
      "catalog-services",
      debouncedQ,
      categoryId,
      selectedTagIds,
      minPrice,
      maxPrice,
      minRating,
      radiusMiles,
      availableDate,
      availableTime,
      sort,
      page,
    ],
    queryFn: () =>
      catalogApi.listServices({
        q: debouncedQ || undefined,
        category_id: categoryId || undefined,
        tag_ids: selectedTagIds.length ? selectedTagIds : undefined,
        min_price: minPrice
          ? Math.round(parseFloat(minPrice) * 100)
          : undefined,
        max_price: maxPrice
          ? Math.round(parseFloat(maxPrice) * 100)
          : undefined,
        min_rating: minRating ? parseFloat(minRating) : undefined,
        radius_miles: radiusMiles ? parseInt(radiusMiles, 10) : undefined,
        available_date: availableDate || undefined,
        available_time: availableTime || undefined,
        sort,
        page,
        page_size: pageSize,
      }),
  });

  const categories = categoriesData?.categories ?? [];
  const tags = tagsData?.tags ?? [];
  const services = data?.services ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.ceil(total / pageSize);
  const history = historyData?.history ?? [];
  const hotKeywords = hotKeywordsData?.keywords ?? [];
  const trending = trendingData?.services ?? [];
  const suggestions = autocompleteData?.terms ?? [];

  // Favorite toggle
  const toggleFavorite = useCallback(
    async (serviceId: string) => {
      const isFav = favoriteIds.has(serviceId);
      setFavoriteIds((prev) => {
        const next = new Set(prev);
        if (isFav) next.delete(serviceId);
        else next.add(serviceId);
        return next;
      });
      try {
        if (isFav) await customerApi.removeFavorite(serviceId);
        else await customerApi.addFavorite(serviceId);
      } catch {
        setFavoriteIds((prev) => {
          const next = new Set(prev);
          if (isFav) next.add(serviceId);
          else next.delete(serviceId);
          return next;
        });
      }
    },
    [favoriteIds],
  );

  // Compare toggle with overflow feedback
  const toggleCompare = useCallback(
    (svc: ServiceSummary) => {
      if (compareHas(svc.id)) {
        compareRemove(svc.id);
        setCompareAlert(null);
      } else {
        const added = compareAdd(svc);
        if (!added) {
          setCompareAlert(
            `Compare is limited to ${MAX_COMPARE} services. Remove one to add another.`,
          );
          setTimeout(() => setCompareAlert(null), 4000);
        } else {
          setCompareAlert(null);
        }
      }
    },
    [compareAdd, compareRemove, compareHas],
  );

  // Autocomplete selection
  const selectSuggestion = (term: string) => {
    setSearchInput(term);
    setShowAutocomplete(false);
    setDebouncedQ(term);
    setPage(1);
  };

  const handleSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!showAutocomplete || suggestions.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedSuggestionIdx((prev) =>
        prev < suggestions.length - 1 ? prev + 1 : 0,
      );
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedSuggestionIdx((prev) =>
        prev > 0 ? prev - 1 : suggestions.length - 1,
      );
    } else if (e.key === "Enter" && selectedSuggestionIdx >= 0) {
      e.preventDefault();
      selectSuggestion(suggestions[selectedSuggestionIdx].term);
    } else if (e.key === "Escape") {
      setShowAutocomplete(false);
    }
  };

  const toggleTag = (tagId: string) => {
    setSelectedTagIds((prev) =>
      prev.includes(tagId)
        ? prev.filter((id) => id !== tagId)
        : [...prev, tagId],
    );
    setPage(1);
  };

  const clearFilters = () => {
    setCategoryId("");
    setSelectedTagIds([]);
    setMinPrice("");
    setMaxPrice("");
    setMinRating("");
    setRadiusMiles("");
    setAvailableDate("");
    setAvailableTime("");
    setSort("newest");
    setPage(1);
  };

  function renderServiceCard(svc: ServiceSummary) {
    const isFav = favoriteIds.has(svc.id);
    const inCompare = compareHas(svc.id);

    return (
      <div
        key={svc.id}
        className="flex flex-col rounded-lg bg-white p-5 shadow-sm transition hover:shadow-md"
      >
        <Link to={`/customer/catalog/${svc.id}`} className="flex-1">
          <h3 className="mb-1 text-base font-semibold text-gray-900">
            {svc.title}
          </h3>
          <p className="mb-1 text-sm text-gray-500">
            {svc.provider.business_name}
          </p>
          <p className="mb-1 text-sm text-gray-500">
            {svc.category?.name ?? "Uncategorized"}
          </p>
          <p className="mb-2 text-lg font-bold text-gray-900">
            {formatPrice(svc.price_cents)}
          </p>
          <div className="mb-2 flex items-center gap-1 text-sm text-yellow-600">
            Rating: {svc.rating_avg}
          </div>
          {svc.tags.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {svc.tags.map((tag) => (
                <span
                  key={tag.id}
                  className="inline-block rounded bg-blue-50 px-2 py-0.5 text-xs text-blue-700"
                >
                  {tag.name}
                </span>
              ))}
            </div>
          )}
        </Link>
        <div className="mt-3 flex items-center gap-2 border-t border-gray-100 pt-3">
          <button
            onClick={() => toggleFavorite(svc.id)}
            className={`rounded-md px-3 py-1.5 text-xs font-medium ${
              isFav
                ? "bg-red-50 text-red-600 hover:bg-red-100"
                : "bg-gray-50 text-gray-600 hover:bg-gray-100"
            }`}
          >
            {isFav ? "Favorited" : "Favorite"}
          </button>
          <button
            onClick={() => toggleCompare(svc)}
            className={`rounded-md px-3 py-1.5 text-xs font-medium ${
              inCompare
                ? "bg-blue-50 text-blue-600 hover:bg-blue-100"
                : "bg-gray-50 text-gray-600 hover:bg-gray-100"
            }`}
          >
            {inCompare ? "In Compare" : "+ Compare"}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="mb-4 text-2xl font-bold text-gray-900">
          Browse Services
        </h1>

        {/* Compare overflow alert */}
        {compareAlert && (
          <div
            role="alert"
            className="mb-4 rounded-md bg-amber-50 p-3 text-sm text-amber-800"
          >
            {compareAlert}
          </div>
        )}

        {/* Search input with autocomplete */}
        <div className="relative mb-4" ref={autocompleteRef}>
          <input
            type="text"
            placeholder="Search services..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onFocus={() => {
              if (suggestions.length > 0 && autocompleteTerm.length >= 2)
                setShowAutocomplete(true);
            }}
            onKeyDown={handleSearchKeyDown}
            className="w-full rounded-md border border-gray-300 px-4 py-2.5 text-sm shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            aria-label="Search services"
            autoComplete="off"
          />
          {showAutocomplete && suggestions.length > 0 && (
            <ul
              role="listbox"
              className="absolute left-0 right-0 z-10 mt-1 max-h-60 overflow-auto rounded-md border border-gray-200 bg-white shadow-lg"
            >
              {suggestions.map((s, idx) => (
                <li
                  key={s.id}
                  role="option"
                  aria-selected={idx === selectedSuggestionIdx}
                  onClick={() => selectSuggestion(s.term)}
                  className={`cursor-pointer px-4 py-2 text-sm ${
                    idx === selectedSuggestionIdx
                      ? "bg-blue-50 text-blue-700"
                      : "text-gray-700 hover:bg-gray-50"
                  }`}
                >
                  {s.term}
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* Hot keywords */}
        {hotKeywords.length > 0 && (
          <div className="mb-4 flex flex-wrap gap-2">
            {hotKeywords.map((kw) => (
              <button
                key={kw.id}
                onClick={() => {
                  setSearchInput(kw.keyword);
                  setShowAutocomplete(false);
                }}
                className="rounded-full bg-blue-50 px-3 py-1 text-xs font-medium text-blue-700 hover:bg-blue-100"
              >
                {kw.keyword}
              </button>
            ))}
          </div>
        )}

        {/* Search history */}
        {history.length > 0 && (
          <div className="mb-4">
            <p className="mb-1 text-xs font-medium uppercase text-gray-400">
              Recent Searches
            </p>
            <div className="flex flex-wrap gap-2">
              {history.slice(0, 10).map((h) => (
                <button
                  key={h.id}
                  onClick={() => {
                    setSearchInput(h.query_text);
                    setShowAutocomplete(false);
                  }}
                  className="rounded-full bg-gray-100 px-3 py-1 text-xs text-gray-600 hover:bg-gray-200"
                >
                  {h.query_text}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Filter toggle & sort */}
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <button
            onClick={() => setShowFilters(!showFilters)}
            className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200"
          >
            {showFilters ? "Hide Filters" : "Show Filters"}
          </button>
          <div className="flex items-center gap-2">
            <label htmlFor="sort-select" className="text-sm text-gray-600">
              Sort:
            </label>
            <select
              id="sort-select"
              value={sort}
              onChange={(e) => {
                setSort(e.target.value);
                setPage(1);
              }}
              className="rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              {SORT_OPTIONS.filter(
                (opt) => opt.value !== "relevance" || debouncedQ,
              ).map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {opt.label}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Filters panel */}
        {showFilters && (
          <div className="mb-6 rounded-lg bg-white p-5 shadow-sm">
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <div>
                <label htmlFor="filter-category" className="block text-sm font-medium text-gray-700">Category</label>
                <select id="filter-category" value={categoryId} onChange={(e) => { setCategoryId(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500">
                  <option value="">All</option>
                  {categories.map((cat) => (<option key={cat.id} value={cat.id}>{cat.name}</option>))}
                </select>
              </div>
              <div>
                <label htmlFor="filter-min-price" className="block text-sm font-medium text-gray-700">Min Price ($)</label>
                <input id="filter-min-price" type="number" min="0" step="0.01" value={minPrice} onChange={(e) => { setMinPrice(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
              </div>
              <div>
                <label htmlFor="filter-max-price" className="block text-sm font-medium text-gray-700">Max Price ($)</label>
                <input id="filter-max-price" type="number" min="0" step="0.01" value={maxPrice} onChange={(e) => { setMaxPrice(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
              </div>
              <div>
                <label htmlFor="filter-min-rating" className="block text-sm font-medium text-gray-700">Min Rating</label>
                <input id="filter-min-rating" type="number" min="0" max="5" step="0.1" value={minRating} onChange={(e) => { setMinRating(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
              </div>
              <div>
                <label htmlFor="filter-radius" className="block text-sm font-medium text-gray-700">Service Area (miles)</label>
                <input id="filter-radius" type="number" min="0" value={radiusMiles} onChange={(e) => { setRadiusMiles(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
              </div>
              <div>
                <label htmlFor="filter-avail-date" className="block text-sm font-medium text-gray-700">Available Date</label>
                <input id="filter-avail-date" type="date" value={availableDate} onChange={(e) => { setAvailableDate(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
              </div>
              {availableDate !== "" && (
                <div>
                  <label htmlFor="filter-avail-time" className="block text-sm font-medium text-gray-700">Available At (time)</label>
                  <input id="filter-avail-time" type="time" value={availableTime} onChange={(e) => { setAvailableTime(e.target.value); setPage(1); }} className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500" />
                </div>
              )}
            </div>

            {tags.length > 0 && (
              <div className="mt-4">
                <p className="mb-2 text-sm font-medium text-gray-700">Tags</p>
                <div className="flex flex-wrap gap-2">
                  {tags.map((tag) => (
                    <label key={tag.id} className="flex items-center gap-1 text-sm text-gray-600">
                      <input type="checkbox" checked={selectedTagIds.includes(tag.id)} onChange={() => toggleTag(tag.id)} className="rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
                      {tag.name}
                    </label>
                  ))}
                </div>
              </div>
            )}

            <div className="mt-4">
              <button onClick={clearFilters} className="text-sm text-blue-600 hover:text-blue-800">Clear All Filters</button>
            </div>
          </div>
        )}
      </div>

      {error && (
        <div className="mb-4 rounded-md bg-red-50 p-3 text-sm text-red-700">{(error as Error).message}</div>
      )}

      {!debouncedQ && trending.length > 0 && (
        <div className="mb-8">
          <h2 className="mb-4 text-lg font-semibold text-gray-900">Trending Services</h2>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{trending.map((svc) => renderServiceCard(svc))}</div>
        </div>
      )}

      <div>
        {debouncedQ && <h2 className="mb-4 text-lg font-semibold text-gray-900">Search Results</h2>}
        {isLoading ? (
          <p className="text-gray-500">Loading...</p>
        ) : services.length === 0 ? (
          <div className="rounded-lg bg-white p-8 text-center shadow-sm"><p className="text-gray-500">No services found.</p></div>
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{services.map((svc) => renderServiceCard(svc))}</div>
            {totalPages > 1 && (
              <div className="mt-6 flex items-center justify-center gap-4">
                <button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-50">Previous</button>
                <span className="text-sm text-gray-600">Page {page} of {totalPages}</span>
                <button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page >= totalPages} className="rounded-md bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-200 disabled:opacity-50">Next</button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

export default CatalogPage;
