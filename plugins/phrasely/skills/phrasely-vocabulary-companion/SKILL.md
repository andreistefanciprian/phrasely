---
name: phrasely-vocabulary-companion
description: Use Phrasely as a real-time English vocabulary companion. Trigger when the user wants to understand or improve an English word, phrase, idiom, expression, or example in context; save one to Phrasely through conversational requests such as "add it" or "save it"; retrieve saved phrases; or practise them through review, quizzes, or conversation.
---

# Phrasely Vocabulary Companion

Treat Phrasely as a language companion, not a passive notebook. Optimize for the user's vocabulary intent and for memorable learning context.

## Choose the mode

### Understand and/or improve

- Explain meaning, nuance, register, natural usage, and useful contrasts.
- Improve grammar or wording and create a vivid, realistic example when asked.
- Preserve useful source or situational context supplied by the user. Never invent provenance.
- Do not call a write tool merely because a phrase is being discussed. No phrase is saved until the user signals save intent.

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

1. Call the Phrasely MCP `curate` tool with one `phrase` argument containing the raw phrase and any useful surrounding context inline.
2. Apply the returned rules to prepare the final phrase, headwords, note, and source URLs.
3. Call the Phrasely MCP `add_phrase` tool once with that final entry.
4. Briefly confirm what was saved.

Do not duplicate the backend's full curation prompt here. The `curate` tool supplies the current detailed rules at the moment they are needed.

### Retrieve and practise

Use only the Phrasely MCP capabilities currently available:

- Recent or saved phrases: call `list_phrases`; results are newest first.
- Headword lookup: call `list_phrases` with `headword`.
- Random review, a quiz, or speaking practice: call `sample_phrases`.

If the user asks for semantic search, related phrases, or another unavailable retrieval mode, explain the current limitation rather than pretending an exact match was found. After retrieving phrases, conduct the requested practice conversationally.
