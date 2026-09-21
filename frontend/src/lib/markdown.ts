import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/common'
import { Marked } from 'marked'
import { markedHighlight } from 'marked-highlight'

const markdown = new Marked({
  breaks: true,
  gfm: true,
})

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
  return typeof html === 'string' ? DOMPurify.sanitize(html) : ''
}
