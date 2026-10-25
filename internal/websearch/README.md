# Public search contract helpers

The code-owned DuckDuckGo Lite endpoint and strict query/limit decoder are shared
by Broker validation, release/destination policy and the WebFetch runtime.
Arguments cannot select a provider endpoint or inject headers/credentials.
This package performs no network operation or authorization itself.
