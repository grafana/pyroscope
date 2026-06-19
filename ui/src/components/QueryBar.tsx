import { Icon } from '@components/core/Icon';
import './QueryBar.css';

export function QueryBar({
  query,
  onQueryChange,
  onRun,
}: {
  query: string;
  onQueryChange: (q: string) => void;
  onRun: (query: string) => void;
}) {
  const handleRun = () => {
    onRun(query);
  };

  return (
    <div className="querybar">
      <input
        className="querybar-input"
        value={query}
        onChange={(e) => onQueryChange(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && handleRun()}
      />

      <button className="querybar-run" onClick={handleRun}>
        <Icon name={'play'} size={10} />
        Run
      </button>
    </div>
  );
}
