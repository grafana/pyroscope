import './Panel.css';

export function Panel({
  title,
  meta,
  progress,
  children,
}: {
  title: string;
  meta?: React.ReactNode;
  // progress is a fraction in [0, 1]. When null/undefined the bar is hidden.
  progress?: number | null;
  children: React.ReactNode;
}) {
  const pct =
    progress == null ? null : Math.max(0, Math.min(1, progress)) * 100;
  return (
    <div className="panel">
      <div className="panel-header">
        <span className="panel-title">{title}</span>
        {meta && <span className="panel-meta">{meta}</span>}
      </div>
      {pct != null && (
        <div className="panel-progress" aria-hidden>
          <div className="panel-progress-bar" style={{ width: `${pct}%` }} />
        </div>
      )}
      <div className="panel-body">{children}</div>
    </div>
  );
}
