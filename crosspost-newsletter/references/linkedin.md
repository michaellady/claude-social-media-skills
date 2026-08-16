# LinkedIn native article

Use the user's signed-in real browser session.

## Preflight

Search the author's activity and article/newsletter archive for the normalized source title, Beehiiv slug, and canonical URL. Open the strongest match. If it is a native article with the same source content, record its public `/pulse/` URL and stop; do not create an accompanying post or another article.

## Draft only when missing

1. Open LinkedIn's article editor and confirm the intended author/publication destination.
2. Enter the source title.
3. Paste `article-linkedin-substack.html`. Verify semantic paragraph and heading counts.
4. Upload the manifest's cover asset to the cover slot.
5. Upload body images in manifest order. LinkedIn may place them together; move figures to the numbered empty paragraph anchors and remove those anchors.
6. Apply captions from the manifest where present.
7. Re-count headings, paragraphs, and figures. Confirm first and last source paragraphs.
8. Draft accompanying copy only if this is a new article. Keep every claim grounded in the source and show the exact copy in the approval prompt.

LinkedIn does not expose a canonical URL field in the article editor. The source link may be included in accompanying copy, but do not claim a canonical tag was set.

## Approval and verification

Show the completed article and accompanying post immediately before the final Publish action. After approval and publish, require both a public `/pulse/` URL and the visible published/congratulations state. Reopen the public article and verify title, body, cover, and body-image count.

If a preflight match already existed, verification consists of reopening that article and recording its public URL; no approval is needed because no side effect occurs.
