# Mirrored toolset surfaces (task 1.3)

Captured 2026-07-15 from the real public servers' documentation. The agenticmarket
duckduckgo/wikipedia servers are behind a registry with undocumented schemas, so
per the design's Open Questions we mirror the canonical OSS equivalents and record
that as the mirror source.

## weather — mirror: github.com/weather-mcp/weather-mcp (17 tools)

Functional: `get_historical_weather`, `search_location`. Rest are stubs.

get_forecast, get_current_conditions, get_alerts, get_historical_weather,
get_weather_summary, search_location, get_air_quality, get_marine_conditions,
get_weather_imagery, get_lightning_activity, get_river_conditions,
get_wildfire_info, check_service_status, save_location, list_saved_locations,
get_saved_location, remove_saved_location.

- get_historical_weather(latitude, longitude, start_date, end_date, hourly?, daily?, timezone?, units?)
- search_location(city_name, latitude?, longitude?, country?)

## pdf-toolkit — mirror: github.com/AryanBV/pdf-toolkit-mcp (22 tools)

Functional: `pdf_create`, `pdf_create_from_markdown`, `pdf_extract_text`,
`pdf_get_metadata`, `pdf_search`. Rest are stubs.

pdf_extract_text, pdf_get_metadata, pdf_get_form_fields, pdf_to_markdown,
pdf_search, pdf_compare, pdf_merge, pdf_split, pdf_delete_pages,
pdf_reorder_pages, pdf_rotate_pages, pdf_flatten, pdf_encrypt,
pdf_add_page_numbers, pdf_embed_qr_code, pdf_create, pdf_create_from_markdown,
pdf_create_from_template, pdf_fill_form, pdf_add_watermark, pdf_embed_image,
pdf_render_pages.

- pdf_create(text, outputPath, pageSize?)
- pdf_create_from_markdown(markdown, outputPath, pageSize?)
- pdf_extract_text(filePath, pages?)
- pdf_get_metadata(filePath)
- pdf_search(filePath, query, caseSensitive?)

## duckduckgo — mirror: github.com/nickclyde/duckduckgo-mcp-server (2 tools)

Functional: `search`, `fetch_content`.

- search(query, max_results=10, region?)
- fetch_content(url, start_index=0, max_length=8000, backend?)

## wikipedia — mirror: github.com/Rudra-ravi/wikipedia-mcp (11 tools)

Functional: `search_wikipedia`, `get_article`, `get_summary`. Rest are stubs.

search_wikipedia(query, limit?), get_article(title), get_summary(title),
get_sections(title), get_links(title), get_coordinates(title),
get_related_topics(title, limit?), summarize_article_for_query(title, query, max_length?),
summarize_article_section(title, section_title, max_length?),
extract_key_facts(title, topic_within_article?, count?), test_wikipedia_connectivity().
