import './Panel.css';

export function Panel({
  title,
  meta,
  children,
}: {
  title: string;
  meta?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="panel">
      <div className="panel-header">
        <span className="panel-title">{title}</span>
        {meta && <span className="panel-meta">{meta}</span>}
      </div>
      <div className="panel-body">{children}</div>
    </div>
  );
}
