# Medium story

Use the user's signed-in real browser session. Medium commonly blocks headless sessions.

## Preflight

Inspect both Published and Drafts under the intended account. Match canonical URL, Beehiiv slug, or exact title. A published match is complete. An equivalent draft may be resumed after its fields are validated; do not create another draft.

## Draft

1. Open a fresh story editor under the intended account.
2. Paste `article-medium.html` into the story body with a real browser paste. It contains the separate cover first and all body images in source order.
3. Enter the source title in Medium's actual in-editor title element, not an outer accessibility wrapper. Enter the source subtitle in the story preview before publishing.
4. Wait for `Saved`. Verify the source paragraph/heading counts and the manifest's cover-plus-body-image count. Avoid programmatic DOM mutation after paste; use real clicks and keystrokes for corrections.
5. Open story settings, expand Advanced Settings, enable “originally published elsewhere,” set the Beehiiv URL, save it, and verify the success message.
6. Add up to five approved topics one at a time. For Enterprise Vibe Code's default AI cluster use: `AI`, `Artificial Intelligence`, `AI Agent`, `Agentic Ai`, `Agents`.
7. Return to the editor and verify title, subtitle, body, cover, every body image, canonical URL, topics, and saved state.

## Approval and verification

Show the completed story immediately before Medium's final Publish button. State the account, exact fields, counts, canonical URL, and topics. After approval, publish once.

Require a public Medium story URL and published state. Reopen the public story and verify exact title, source-faithful body, cover, body-image count, and a canonical link element pointing to Beehiiv. Autosave alone is not publication; edits to an existing story require `Save and publish`.
