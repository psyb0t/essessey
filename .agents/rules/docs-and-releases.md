# Docs and releases

Prefix: DOC.

- **DOC1:** A change to behavior or public API updates the matching page in `docs/` in the same change. The root `README.md` is the front door: a short pitch, a quick look, and links to the site with full `https://psyb0t.github.io/essessey/` URLs. Package READMEs (`sse/`, `nats/`, `ws/`, `elelemstream/`) stay short pointers to their pages.
- **DOC2:** Every `docs/` page has front matter with a unique `title` and a `description` of 50 to 170 characters, and exactly one `#` heading. Page-to-page links use the relative `.md` path; links to repository files use the full `https://github.com/psyb0t/essessey/blob/main/...` URL.
- **DOC3:** After a docs change, run `make docs` and check the built site before pushing. The output directory is `DOCS_SITE` in the `Makefile` and is gitignored.
- **DOC4:** Prose has no em dashes, uses "directory" for filesystem locations, and keeps each paragraph on one source line.
- **DOC5:** Releases are annotated `v*` tags on a commit that already passed CI on `main`. `CHANGELOG.md` gets a section per version that says whether the API changed. Pre-1.0, a minor bump may break the API and says so.
