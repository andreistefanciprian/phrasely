---
name: phrasely
description: Use Phrasely as a real-time English vocabulary companion. Trigger when the user wants to understand, correct, compare, or practise an English word, phrase, idiom, expression, or example in context; save one through conversational requests such as "add it" or "save it"; retrieve saved phrases; or review and quiz their private Phrasely collection.
---

# Phrasely

Treat Phrasely as a language companion, not a passive notebook. Optimize for the user's vocabulary intent, preserve their real-world context, and use only the MCP capabilities currently available.

## Choose the tool

- `explore_phrase` returns learning instructions for a new word or expression. Call it once for each newly introduced target when the user wants meaning, nuance, correction, comparison, or memorable examples. It never saves data.
- `render_phrase_choices` presents one to four complete save-ready candidates. Call it once after exploration. It never saves data; its Save buttons call `add_phrase` themselves.
- `add_phrase` saves one finished entry. Call it only after clear save intent, once per entry the user asked to save.
- `list_phrases` returns saved phrases newest first, optionally filtered by a case-insensitive partial headword match. Use it for recent phrases, browsing, and headword lookup.
- `sample_phrases` returns a random sample for review, quizzes, or speaking practice. Use a count from 1 to 10; do not use it when the user wants recent or complete results.

Do not substitute a different Phrasely tool when the requested capability is unavailable. Phrasely currently has no MCP tool for semantic search, related phrases, editing, or deleting.

## Understand or improve an expression

- Pass the raw expression and any useful surrounding context to `explore_phrase`. If the user provides a paragraph, include only enough context to preserve the intended meaning and provenance.
- For a follow-up about the same expression, continue without calling `explore_phrase` again unless the user supplies materially different context or introduces another target.
- If the user asks about several distinct expressions, explore them separately and keep each expression's choices separate. Do not combine unrelated targets into one headword list.
- Identify the likely target expression before teaching it. If multiple targets remain equally plausible and choosing one would change the answer, ask which one they mean.
- Apply the returned instructions conversationally. Explain meaning, nuance, register, natural usage, useful contrasts, and common constructions without turning the answer into a dictionary entry.
- Correct grammar or awkward wording when useful while preserving the user's meaning, subject matter, tone, and supplied provenance. Never invent where a phrase came from.
- Attribute opinions, controversial claims, political arguments, and religious arguments to their speaker or source rather than presenting them as objective facts.
- Do not call `add_phrase` merely because an expression is being discussed. Explanation, correction, comparison, and exploration are read-only intents.

After the explanation, generate the refined original context and useful memorable alternatives. Prefer vivid, natural examples over generic dictionary-style sentences and include a simple everyday context when it improves learning.

## Build save-ready entries

Prepare every candidate according to the `add_phrase` field descriptions:

- `phrase`: Write polished, natural English, usually as one memorable sentence. Put a short plain-English meaning in parentheses immediately after each taught word or expression. Use no Markdown inside the saved phrase.
- `headwords`: Use raw canonical taught forms only—no definitions, parentheses, quotation marks, or commentary. Treat an idiom or fixed expression as one headword. Keep fixed particles or prepositions but remove replaceable complements; for example, both “unbeknownst to me” and “unbeknownst to the team” use `unbeknownst to`.
- `note`: Use one to three concise sentences about nuance, tone, register, collocations, grammar, or a genuinely established origin. Do not repeat the phrase unnecessarily, speculate, or invent etymology.
- `source_urls`: When supplied, include exactly one Merriam-Webster URL per headword in the same order. Use the actual dictionary lookup form, which may be a base verb or noun phrase rather than the saved headword. Supply a complete one-to-one list, or omit `source_urls` entirely if any required lookup is uncertain; never submit a partial list.

Reuse the exact same canonical `headwords` and aligned `source_urls` across every choice for one target expression. Only the surrounding context and replaceable parts may vary.

## Present choices

- Call `render_phrase_choices` once with one to four final candidates after exploration. Include the refined original context when useful and add memorable alternatives rather than near-duplicates.
- Mark at most one especially useful learning context as recommended. Do not mark one merely to fill the field.
- Treat rendering as presentation only. Never imply that a rendered choice was saved.
- Use the UI as the primary presentation and avoid repeating every full candidate in prose.
- If interactive UI is unavailable, present the candidates as a numbered list with the same phrase, headwords, and concise note so the user can refer to a choice naturally.
- Finish an exploration with exactly one compact aside labelled `One useful connection:` following the priority and boundaries returned by `explore_phrase`.

Do not render choices after a direct, unambiguous instruction to save an already identified phrase. Save that entry immediately.

## Resolve save intent

Resolve conversational references against the most recent phrase or choice set:

- Treat “add it”, “save it”, “add this”, “save this”, “put this in Phrasely”, and equivalent direct requests as confirmation. Do not ask for confirmation again.
- Resolve “number 3”, “the last one”, or similar references against the most recent numbered choices.
- Resolve “it”, “this”, or “that one” only when the user has one phrase clearly in focus: the only phrase presented or the user's explicit selection. When several choices remain visible, a recommendation alone does not disambiguate the write; require a number, the phrase itself, or an explicit reference such as “the recommended one”.
- Treat a soft signal such as “that's worth keeping” as save intent only when exactly one phrase is clearly in focus. Otherwise ask which phrase to save.
- If multiple phrases remain equally plausible, ask the user to identify the intended one before writing.
- If the user explicitly asks to save several identified entries, call `add_phrase` once for each entry. Do not silently collapse them into one record.
- Do not call `explore_phrase` solely to prepare an unambiguous save. Construct the finished entry from the selected phrase and call `add_phrase` directly.
- Do not assume that a silent UI Save click is visible in conversation. The UI confirms its own result. Avoid calling `add_phrase` again only when the user says the click succeeded or the host exposes the successful tool result.

After an observed `add_phrase` success, briefly report the saved phrase and its headword or headwords. Do not expose internal identifiers or claim that anything else was changed.

`add_phrase` is not an edit or upsert operation. Do not use it to imitate an update. If a save has an ambiguous timeout or transport failure, do not retry automatically because that could create a duplicate; say that completion could not be confirmed and let the user choose whether to check or retry.

## Retrieve saved phrases

- Use `list_phrases` without a filter for the collection or recent entries. Results are newest first.
- Use `list_phrases` with `headword` for headword lookup. The match is case-insensitive and partial; describe an empty result as no matching saved headword, not proof that the expression never appears anywhere in a phrase.
- Treat returned phrases as the user's saved wording. Preserve them when summarizing or practising unless the user asks to improve them.
- The result contains only each saved phrase and its headwords. Do not claim access to notes, source URLs, IDs, timestamps, or exact save dates.
- Do not invent pagination, semantic matching, related-phrase ranking, or an exact match when the tool does not provide it.

If the user requests semantic search, related phrases, editing, deletion, or another unavailable collection operation, explain the limitation plainly and offer the closest supported action only when it is genuinely useful.

## Review and practise

- Use `sample_phrases` for random review, quizzes, recall exercises, and speaking practice. Respect an explicit count and cap it at 10.
- Do not describe a random sample as recent, representative, or exhaustive.
- Build the requested exercise from the returned phrase and headwords. For a quiz, withhold the answer until the user responds unless they ask to see it immediately.
- Keep practice grounded in the user's saved material. You may create a new conversational prompt or example, but do not imply that the new wording is already saved.
- Use `list_phrases` instead when the user asks to practise a specific headword or browse all matching entries.

## Authentication and failures

`explore_phrase` and `render_phrase_choices` do not require access to the user's collection. `list_phrases`, `sample_phrases`, and `add_phrase` require the connected Phrasely account.

- If authentication is requested, let the user connect Phrasely and retry only after access is available.
- Do not fall back to invented collection data when authentication, authorization, availability, or tool execution fails.
- Do not claim a write succeeded unless `add_phrase` returned success.
- Report failures briefly with the affected action and a useful next step. Never expose bearer tokens, raw internal IDs, or diagnostic payloads.
