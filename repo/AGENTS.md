# Developer Rulebook

This file is the repo-local engineering rulebook for `slopmachine` projects.

## Scope

- Treat the current working directory as the project.
- Ignore parent-directory workflow files unless the user explicitly asks you to use them.
- Do not treat workflow research, session exports, or sibling directories as hidden implementation instructions.
- Do not make the repo depend on parent-directory docs or sibling artifacts for startup, build/preview, configuration, verification, or basic project understanding.

## Working Style

- Operate like a strong senior engineer.
- Read the code before making assumptions.
- Work in meaningful vertical slices.
- Do not call work complete while it is still shaky.
- Reuse and extend shared cross-cutting patterns instead of inventing incompatible local ones.

## Runtime And Verification

- Keep one primary documented runtime command and one primary broad test command: `./run_tests.sh`.
- Follow the original prompt and existing repository first for the runtime stack.
- Prefer the fastest meaningful local verification for the changed area during ordinary iteration.
- Do not rerun broad runtime/test commands on every small change.
- For web projects, default the runtime contract to `docker compose up --build` unless the prompt or existing repository clearly dictates another model.
- When `docker compose up --build` is not the runtime contract, provide `./run_app.sh` as the single primary runtime wrapper.
- If the project has database dependencies, keep `./init_db.sh` as the only project-standard database initialization path.

## Documentation Rules

- Keep `README.md` and any codebase-local docs accurate.
- The README must explain what the project is, what it does, how to run it, how to test it, the main repo contents, and any important information a new developer needs immediately.
- The README must clearly document whether the primary runtime command is `docker compose up --build` or `./run_app.sh`.
- The README must clearly document `./run_tests.sh` as the broad test command.
- The README must stand on its own for basic codebase use.
- Keep `README.md` as the only documentation file inside the repo unless the user explicitly asks for something else.
- Treat `README.md` as the primary documentation surface inside the repo.
- The repo should be statically reviewable by a fresh reviewer: entry points, routes, config, test commands, and major module boundaries should be traceable from repository artifacts.
- If the project uses mock, stub, fake, interception, or local-data behavior, the README must disclose that scope accurately.
- If mock or interception behavior is enabled by default, the README must say so clearly.
- Feature flags, debug/demo surfaces, default enabled states, and mock/interception defaults must be disclosed in `README.md` when they exist.
- Do not let a mock-only or local-data-only project look like undisclosed real backend or production integration.
- Do not hide missing failure handling behind fake-success paths.

## Secret And Runtime Rules

- Do not create or keep `.env` files anywhere in the repo.
- Do not rely on `.env`, `.env.local`, `.env.example`, or similar files for project startup.
- Do not hardcode secrets.
- If runtime env-file format is required, generate it ephemerally and do not commit or package it.
- Do not hardcode database connection values or database bootstrap values anywhere in the repo.
- For Dockerized web projects, `docker compose up --build` should work without any manual `export ...` step.
- For Dockerized web projects, prefer a dev-only runtime bootstrap script that is invoked automatically by the Docker startup path to generate or inject local-development runtime values.
- That bootstrap path must not use checked-in `.env` files or hardcoded runtime values.
- If such a bootstrap script exists, document in the script and in `README.md` that it is for local development bootstrap only and is not the production secret-management path.

## Product Integrity Rules

- Do not leave placeholder, setup, debug, or demo content in product-facing UI.
- If a real user-facing or admin-facing surface is required, build that surface instead of bypassing it with API shortcuts.
- Treat missing real surfaces as incomplete implementation.

## Rulebook Files

- Do not edit `AGENTS.md` or other workflow/rulebook files unless explicitly asked.
