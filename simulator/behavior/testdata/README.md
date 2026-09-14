These synthetic compatibility records were generated using unchanged code at
54b7ab079723af9b96b5312cb7983652d92e5ba9 before the #41 implementation. They cover
all nine `human-actions.v1` helpers, their complete post-choice actor state,
decision, and unknown outcome. `TestFrozenLegacyActionsV1` reconstructs the same
inputs against current code and pins the original complete canonical JSON bytes.
The generation inputs are visible in that test. This is a compatibility fixture,
not a historical human observation or a scientific validation dataset.

`app/hws/testdata/legacy-cognitive-v1.txt` was encoded by that same baseline's
CognitiveCheckpoint.Encode with the final third_party_support record, a fresh actor b, no
commitments, and a synthetic all-a model hash. Its decoder/re-encoder test pins
the original compressed bytes. New v2 code must never reinterpret these records.

`legacy-context-actions-v1.json` was generated separately against the same
unchanged baseline, with nonzero observer history, supplied relationship context,
and beliefs. The original fixture bytes above remain unchanged. Its explicit
inputs are the contextual branch of verifyLegacyChoices; coefficient mutations
for trust, disclosure and beliefs must change the frozen decisions. These pins
exercise behavior, not just decoder acceptance or a codec size boundary.

`legacy-competing-actions-v1.json` adds an eligible Ask (or Observe when Ask is
already primary), alongside WAIT and the primary offer, using the same nonzero
context. A midpoint draw replaces the forced upper-tail draw. It was generated
against the same unchanged baseline; both earlier fixture files remain intact.
The frozen bytes pin scores, probabilities, selected actions and resulting state.
A 0.4-to-0.9 self-disclosure trust-coefficient mutation is explicitly caught.
