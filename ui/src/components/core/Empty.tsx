import './Empty.css';

export function Empty({ message = 'No data' }: { message?: string }) {
  return (
    <div className="empty">
      <span className="empty-message">{message}</span>
    </div>
  );
}
