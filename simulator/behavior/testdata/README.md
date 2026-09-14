These synthetic compatibility records were generated using unchanged code at
54b7ab079723af9b96b5312cb7983652d92e5ba9 before the #41 implementation. They cover
all nine `human-actions.v1` helpers, their complete post-choice actor state,
decision, and unknown outcome. `TestFrozenLegacyActionsV1` reconstructs the same
inputs against current code and pins the original complete canonical JSON bytes.
The generation inputs are visible in that test. This is a compatibility fixture,
not a historical human observation or a scientific validation dataset.

`app/hws/testdata/legacy-cognitive-v1.txt` was encoded by that same baseline's
CognitiveCheckpoint.Encode with the final WAIT record, a fresh actor b, no
commitments, and a synthetic all-a model hash. Its decoder/re-encoder test pins
the original compressed bytes. New v2 code must never reinterpret these records.
