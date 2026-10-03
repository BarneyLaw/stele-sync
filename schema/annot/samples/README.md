# FreeDraw PDF 0.13.3 samples

Captured on 2026-10-04 (Singapore) from the actual installed 0.13.3 plugin in
Obsidian 1.13.7 on Linux, using an isolated temporary profile and the synthetic
one-page PDF in this directory. No lecture or personal vault content is included.
These are captured artifacts, not generated contract goldens; do not regenerate
them as part of `make fixtures`.
The PDF is marked binary in Git so line-ending conversion cannot invalidate its
cross-reference byte offsets (including required fixed-width trailing spaces).

Pen/highlighter/shape input went through the plugin's pointer event handlers.
Text used its inline editor; the embedded image is a generated 16×16 PNG.
Eraser and page operations invoked the plugin's session methods, including the
page-trash confirmation button. The plugin itself serialized every sample.

| Sample | Action |
| --- | --- |
| 01-tools | Pen, highlighter, rectangle, Unicode/HTML-like text, embedded image |
| 02-segment-erase | Split the pen stroke; first fragment retains its id |
| 03-object-erase | Remove the highlighter |
| 04-hidden-template | Add a grid template and hide the real PDF page |
| 05-added-page | Restore the real page, insert a ruled page and draw on page 2 |
| 06-trash | Move the added page and its annotation into removedPages |
| 07-restored | Restore the page with the same annotation id |
| 08-renamed | Rename Synthetic.pdf to Synthetic-renamed.pdf |

Go parsed, materialized and reparsed every capture. All eight canonical outputs
were then reopened in the actual plugin, retaining the expected live annotation,
added-page and trash counts. This is automated application verification, not a
desktop/Android manual usability matrix. See [the verification record](../../../docs/annot-verification.md).

For a manual check, copy Synthetic.pdf into a disposable vault with FreeDraw
0.13.3, place one sample beside it as Synthetic.pdf.annot.json, then open it.
For sample 08, name both files Synthetic-renamed.pdf[.annot.json]. Close the PDF
and allow pending saves to finish before swapping samples.
