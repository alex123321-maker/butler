# Review document serialization

Before committing the four newly authored independent reviews, their author
removed exactly one extra LF at EOF from each document. One final LF remains.
Content, decisions and evidence were unchanged. Historical gate references to
pre-format SHA256 values remain historical. These documents were not part of
the executed a3d66736 source snapshot.

The original author bytes are reproduced by appending exactly one LF byte to
the committed document. Both SHA256 columns were checked before commit.

| Document under docs/ai | Historical author SHA256 | Committed SHA256 |
| --- | --- | --- |
| memory-eval-v2-renewed-five-source-readiness-20261015.md | 61ce41c9624a84962877f3018c39e5925984413f68a754b82296e4c3c569ed7b | 2c5672a3ff98cacbd99235f25d1ec3e2e9c73ecdb173df3175c2a8cfc1812c61 |
| memory-eval-v2-renewed-five-prepared-auth-review-20261015.md | 26b2708e170ffe192d0818f015bb7d32eac42105af53b5c4782d6953d5a0ac89 | 1c6270038bb57cf905bbc271d7986d2c6570772215aef5300a5a4d9881feacd5 |
| memory-eval-v2-renewed-five-native-readiness-20261015.md | 6feea6cc29c8a34ffdfd006006bb55ed66bca0db18ad4b05658bcddbefce2344 | 7d91db27fca951b557bf35ca0bd5ee467a3f287e5084d707188b5347093f7881 |
| memory-eval-v2-renewed-five-final-independent-review-20261015.md | 30b927d0d30c2eab06ee70af7f755575a74c8891bf6ff2096afe2323b4fc9d4b | f2d8539f049a9542808bfccbeab34b5ec85813b2e7dde4bbcdd7032d7703f58a |
