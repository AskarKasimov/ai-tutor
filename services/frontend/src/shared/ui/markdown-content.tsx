import { useMemo } from 'react'
import Markdown, { type Components } from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import { MarkdownCodeBlock } from './markdown-code-block'
import styles from './markdown-content.module.scss'

const plugins = [remarkGfm, remarkBreaks]
const defaultComponents: Components = {
  pre: ({ children }) => <MarkdownCodeBlock>{children}</MarkdownCodeBlock>,
  table: ({ children }) => (
    <div className={styles.tableWrapper}>
      <table>{children}</table>
    </div>
  ),
  // Content headings sit below the page and section headings.
  h1: ({ children }) => <h3>{children}</h3>,
  h2: ({ children }) => <h4>{children}</h4>,
  h3: ({ children }) => <h5>{children}</h5>,
  h4: ({ children }) => <h6>{children}</h6>,
  h5: ({ children }) => <h6>{children}</h6>,
  h6: ({ children }) => <h6>{children}</h6>,
}

function normalizeGluedFences(text: string): string {
  let fence: { marker: string; length: number } | undefined
  return text
    .split('\n')
    .map((line) => {
      if (fence) {
        const closing = /^ {0,3}(`{3,}|~{3,})[ \t\r]*$/.exec(line)
        if (
          closing &&
          closing[1][0] === fence.marker &&
          closing[1].length >= fence.length
        ) {
          fence = undefined
        }
        return line
      }

      const opening = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line)
      if (opening && (opening[1][0] === '~' || !opening[2].includes('`'))) {
        fence = { marker: opening[1][0], length: opening[1].length }
        return line
      }

      // Repair only an opening fence at the end of prose. Leave inline code
      // spans and indented code alone; never rewrite an existing fenced block.
      const glued = /^([^`]+)(`{3,})([a-zA-Z0-9_+-]*)[ \t\r]*$/.exec(line)
      if (!glued || /^ {4}|^\t/.test(line) || !glued[1].trim()) return line
      fence = { marker: '`', length: glued[2].length }
      return `${glued[1]}\n${line.slice(glued[1].length)}`
    })
    .join('\n')
}

// Markdown is authored at the source. Keep code and escapes intact; raw HTML
// stays disabled by react-markdown's default renderer.
export function MarkdownContent({
  text,
  className,
  components,
}: {
  text: string
  className?: string
  components?: Components
}) {
  const normalized = useMemo(() => normalizeGluedFences(text), [text])

  if (!normalized.trim()) return null
  return (
    <div className={`${styles.content} ${className ?? ''}`}>
      <Markdown
        remarkPlugins={plugins}
        components={
          components
            ? { ...defaultComponents, ...components }
            : defaultComponents
        }
      >
        {normalized}
      </Markdown>
    </div>
  )
}
