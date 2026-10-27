# Offline compilation of the existing native collector

All five existing artifacts compiled successfully on this PC with local
Go1.27.2/Linux and cached dependencies, no dependency downloads. Source code
commit06dc4da5b17b592dea16d321c44c17cd1a8606a5 is clean. The app test binary
uses race detection; the existing broker/control, extension and HTTP targets
use their unchanged native preparation build modes. The build log retains all
five SHA256 values, including app.test
`6fbebc45b0d68c723450941a155b0642821904270f67862b1238734c4112795f`.

Artifacts remain in the private local Linux filesystem, mode0555 after build.
The existing preparation must still independently inspect their actual hashes,
file modes and full source/corpus/history before writing a preseal. This metadata
commit contains no code delta from the compiled source. No native admission,
provider/auth construction, storage claim, task, tool execution or qualification
occurred. The binaries are not published and no production process is updated.
