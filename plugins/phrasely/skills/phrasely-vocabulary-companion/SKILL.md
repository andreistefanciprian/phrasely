---
name: phrasely-vocabulary-companion
description: Use Phrasely as a real-time English vocabulary companion. Trigger when the user wants to understand or improve an English word, phrase, idiom, expression, or example in context; save one to Phrasely through conversational requests such as "add it" or "save it"; retrieve saved phrases; or practise them through review, quizzes, or conversation.
---

# Phrasely Vocabulary Companion

Treat Phrasely as a language companion, not a passive notebook. Optimize for the user's vocabulary intent and for memorable learning context.

## Choose the mode

### Understand and/or improve

- For each newly introduced word or expression, call the Phrasely MCP `explore_phrase` tool once with the raw phrase and any useful surrounding context inline, then apply its returned learning instructions conversationally.
- For follow-up questions about the same expression, continue the conversation without calling `explore_phrase` again unless the user supplies materially different context or introduces a new target expression.
- Explain meaning, nuance, register, natural usage, and useful contrasts.
- Improve grammar or wording and create a vivid, realistic example when asked.
- Preserve useful source or situational context supplied by the user. Never invent provenance.
- Do not call a write tool merely because a phrase is being discussed. No phrase is saved until the user signals save intent.
- After generating the refined original context and final memorable alternatives, prepare complete save-ready fields for each choice following the `add_phrase` field descriptions, then call `render_phrase_choices` once. Mark at most one especially useful learning context as recommended. Use the UI as the primary presentation of the full candidates and avoid duplicating them in prose.
- Identify one canonical taught headword list for the target expression and reuse it exactly across every choice. Keep fixed particles or prepositions, but exclude replaceable complements: `unbeknownst to me` and `unbeknownst to the engineering team` both use the headword `unbeknownst to`.
- `render_phrase_choices` is presentation-only. Its Save buttons express the user's save intent and call `add_phrase`; do not call `add_phrase` merely because the choices were rendered.
- If interactive UI is unavailable, number two or more candidates so the user can select one conversationally. Do not let UI availability block the learning workflow.

### Save

Resolve conversational references against the phrase currently in focus.

- When presenting two or more candidate phrases or examples, number them so the user can refer to one naturally. Clearly identify the recommended phrase when there is one.
- Resolve references such as "the third phrase", "number 3", or "the last one" against the most recent numbered list.
- Resolve "it", "this", or "that one" to the single phrase currently in focus: the only phrase presented, the assistant's clearly identified recommendation, or the phrase most recently selected by the user.
- Treat direct requests such as "add it", "save it", "add this", "save this", "put this in Phrasely", and "let's add this one" as confirmation. Do not ask for confirmation again.
- Treat a soft signal such as "that's worth keeping" or "great one" as save intent only when exactly one phrase is clearly in focus. Otherwise ask which phrase to save.
- If multiple phrases remain equally plausible, ask which phrase the user wants to save.
- A request only to explain, define, compare, or rewrite is not save intent.

For a confirmed save:

1. Construct the finished phrase, headwords, note, and source URLs locally from the phrase the user supplied or selected, following the `add_phrase` field descriptions.
2. Call the Phrasely MCP `add_phrase` tool once with that final entry.
3. Briefly confirm what was saved.

Do not call `explore_phrase` solely to prepare a save. It is a learning tool, not a prerequisite for `add_phrase`.
Do not call `render_phrase_choices` after a direct, unambiguous save instruction; save the identified phrase immediately.

### Retrieve and practise

Use only the Phrasely MCP capabilities currently available:

- Recent or saved phrases: call `list_phrases`; results are newest first.
- Headword lookup: call `list_phrases` with `headword`.
- Random review, a quiz, or speaking practice: call `sample_phrases`.

If the user asks for semantic search, related phrases, or another unavailable retrieval mode, explain the current limitation rather than pretending an exact match was found. After retrieving phrases, conduct the requested practice conversationally.
