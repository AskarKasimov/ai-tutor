import Markdown, { type Components } from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import styles from './markdown-content.module.scss'

const plugins = [remarkGfm, remarkBreaks]
const defaultComponents: Components = {
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
  if (!text.trim()) return null
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
        {text}
      </Markdown>
    </div>
  )
}
