# Isolated Broker readiness

Build on laba and run with the native stand's exact environment. It refuses
other addresses, calls only ListTools and exports a count/success boolean.
No credentials, tool execution, model request or authorization bypass.
Both HTTP and gRPC internal service names belong in NO_PROXY.
