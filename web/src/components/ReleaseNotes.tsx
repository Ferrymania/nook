import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

export function ReleaseNotes({ children }: { children: string }) {
  return (
    <div className="release-notes mt-3 max-h-64 overflow-y-auto break-words text-sm leading-7 text-[var(--app-subtle)]">
      <Markdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        components={{
          a: ({ href, children }) => <a href={href} target="_blank" rel="noreferrer noopener">{children}</a>,
        }}
      >
        {children}
      </Markdown>
    </div>
  )
}
