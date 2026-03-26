export interface BookmarkData {
  id: string
  name: string
  url: string
}

interface Props {
  bookmark: BookmarkData
}

export function BookmarkWidget({ bookmark }: Props) {
  return (
    <a
      className="bookmark-widget"
      href={bookmark.url}
      target="_blank"
      rel="noopener noreferrer"
    >
      <div className="bookmark-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="12" cy="12" r="10" />
          <line x1="2" y1="12" x2="22" y2="12" />
          <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
        </svg>
      </div>
      <div className="bookmark-info">
        <div className="bookmark-name">{bookmark.name}</div>
        <div className="bookmark-url">{bookmark.url}</div>
      </div>
      <div className="bookmark-arrow">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
          <polyline points="15 3 21 3 21 9" />
          <line x1="10" y1="14" x2="21" y2="3" />
        </svg>
      </div>
    </a>
  )
}
