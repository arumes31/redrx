# Security & Code Quality Learnings

## High Cyclomatic Complexity in API Routes
- **Issue:** The `/shorten` API endpoint had a complexity score of 46 (Grade F), making it difficult to maintain and test.
- **Solution:** Extracted logic into discrete, single-purpose helper functions:
    - `_parse_iso_datetime`: Handles date parsing.
    - `_resolve_short_code`: Handles collision detection and generation.
    - `_validate_rotate_targets`: Validates list-based inputs.
    - `_validate_basic_params`: Handles required field checks.
    - `_process_url_timestamps`: Orchestrates date/expiry logic.
    - `_create_new_url`: Encapsulates DB insertion.
    - `_build_shorten_response`: Standardizes API output.
- **Outcome:** Reduced `shorten` complexity to 7 (Grade B).
- **Prevention:** Use tools like `radon` during development to monitor function complexity and refactor early when scores exceed 10-15.
