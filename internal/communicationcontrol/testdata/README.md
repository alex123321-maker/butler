# C01 communication contract corpus

`v1.json` freezes all four model formats, the three blocking question types,
optional/free-text answers, multi-choice Submit, optional expiry, ordered image
refs and the nine canonical report block types. Preference is DATA, never payment
approval. False `wait_for_response`, forms and raw HTML are negative decoder cases
in `contracts_test.go`. Fan-out, first response and result obligations are owning
PostgreSQL tests, not JSON claims.

The report includes a manually authored native `sendRichMessage` expectation:
`InputRichMessage.blocks`, native photo/document media objects, list, quotation,
preformatted text and table cells. Artifact bytes remain private multipart refs.
The canonical report is kept alongside it; a URL/PDF/summary is not a native
report. This fixture is not a renderer, API request or live Telegram acceptance;
those belong to C06. The current Telegram adapter is unchanged.

Primary schema references rechecked 2026-10-21:
[InputRichMessage](https://core.telegram.org/bots/api#inputrichmessage),
[sendRichMessage](https://core.telegram.org/bots/api#sendrichmessage),
[InputRichBlockPhoto](https://core.telegram.org/bots/api#inputrichblockphoto),
[InputRichBlockDocument](https://core.telegram.org/bots/api#inputrichblockdocument),
[InputRichBlockTable](https://core.telegram.org/bots/api#inputrichblocktable).
