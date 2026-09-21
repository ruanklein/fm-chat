import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import { Marked } from 'marked'
import { markedHighlight } from 'marked-highlight'

const markdown = new Marked({
  breaks: true,
  gfm: true,
})

const copyIcon = '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>'

markdown.use(
  markedHighlight({
    emptyLangClass: 'hljs',
    langPrefix: 'hljs language-',
    highlight(code, language) {
      const normalizedLanguage = language && hljs.getLanguage(language) ? language : 'plaintext'
      return hljs.highlight(code, { language: normalizedLanguage }).value
    },
  }),
)

export function renderMarkdown(source: string): string {
  const html = markdown.parse(source)
  if (typeof html !== 'string') return ''

  const sanitized = DOMPurify.sanitize(html)
  return sanitized.replace(
    /<pre>([\s\S]*?)<\/pre>/g,
    '<pre><button type="button" class="copy-code-button" data-copy-code aria-label="Copy code">' + copyIcon + '</button>$1</pre>',
  )
}
