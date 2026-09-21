import DOMPurify from 'dompurify'
import { Marked } from 'marked'

const markdown = new Marked({
  breaks: true,
  gfm: true,
})

export function renderMarkdown(source: string): string {
  const html = markdown.parse(source)
  return typeof html === 'string' ? DOMPurify.sanitize(html) : ''
}
