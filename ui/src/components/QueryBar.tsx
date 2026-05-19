import { useEffect, useRef, useState } from 'react';
import { Icon } from '@components/core/Icon';
import { SuggestionList } from '@components/SuggestionList';
import { useLabelSuggestions } from '@hooks/useLabelSuggestions';
import { applySuggestion } from '../queryLang';
import './QueryBar.css';

export function QueryBar({
  query,
  onQueryChange,
  onRun,
  start,
  end,
  tenantID,
}: {
  query: string;
  onQueryChange: (q: string) => void;
  onRun: (query: string) => void;
  start: number;
  end: number;
  tenantID?: string;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [cursor, setCursor] = useState(0);
  const [focused, setFocused] = useState(false);
  const [escaped, setEscaped] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  // Gates the dropdown on the user having actually typed or accepted a
  // suggestion since the input was last focused. Without this, clicking
  // into the input would pop the dropdown solely from the cursor position,
  // which is noisy when the user just wants to read or move the caret.
  const [interacted, setInteracted] = useState(false);

  // Carries the caret position that should be applied to the input after the
  // next commit. We use a ref instead of state so the post-commit effect can
  // clear it without a cascading setState.
  const pendingCursorRef = useRef<number | null>(null);

  const [lastRun, setLastRun] = useState<string | null>(null);
  const dirty = lastRun === null || lastRun !== query;

  const { context, suggestions, definitelyEmpty } = useLabelSuggestions({
    query,
    cursor,
    start,
    end,
    tenantID,
  });

  // Clamp the highlight against the current suggestions length. This avoids
  // a setState-in-effect reset; if the list shrinks below the previous
  // index we just fall back to the end of the list.
  const safeHighlight =
    suggestions.length === 0
      ? 0
      : Math.min(highlightedIndex, suggestions.length - 1);

  const open =
    focused &&
    interacted &&
    !escaped &&
    (suggestions.length > 0 || definitelyEmpty);

  useEffect(() => {
    if (pendingCursorRef.current === null) return;
    const el = inputRef.current;
    if (!el) return;
    const pos = pendingCursorRef.current;
    el.setSelectionRange(pos, pos);
    pendingCursorRef.current = null;
  }, [query]);

  const handleRun = () => {
    setLastRun(query);
    onRun(query);
  };

  const accept = (item: string) => {
    const { next, cursor: newCursor } = applySuggestion(query, context, item);
    pendingCursorRef.current = newCursor;
    onQueryChange(next);
    setCursor(newCursor);
    setHighlightedIndex(0);
    setEscaped(false);
    setInteracted(true);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
      e.preventDefault();
      const dir = e.key === 'ArrowDown' ? 1 : -1;
      const n = suggestions.length;
      setHighlightedIndex((safeHighlight + dir + n) % n);
      return;
    }
    if (e.key === 'Enter') {
      if (open && suggestions[safeHighlight]) {
        e.preventDefault();
        accept(suggestions[safeHighlight]);
        return;
      }
      handleRun();
      return;
    }
    if (e.key === 'Escape' && open) {
      e.preventDefault();
      setEscaped(true);
    }
  };

  return (
    <div className="querybar">
      <div className="querybar-input-wrap">
        <input
          ref={inputRef}
          className="querybar-input"
          value={query}
          onChange={(e) => {
            setEscaped(false);
            setInteracted(true);
            setHighlightedIndex(0);
            onQueryChange(e.target.value);
            setCursor(e.target.selectionStart ?? e.target.value.length);
          }}
          onSelect={(e) => {
            setHighlightedIndex(0);
            setCursor(e.currentTarget.selectionStart ?? cursor);
          }}
          onFocus={(e) => {
            setFocused(true);
            setCursor(e.currentTarget.selectionStart ?? 0);
          }}
          onBlur={() => {
            setFocused(false);
            setInteracted(false);
          }}
          onKeyDown={handleKeyDown}
        />
        {open && (
          <SuggestionList
            items={suggestions}
            highlightedIndex={safeHighlight}
            onHover={setHighlightedIndex}
            onAccept={accept}
          />
        )}
      </div>

      <button className="querybar-run" onClick={handleRun}>
        <Icon name={dirty ? 'play' : 'refresh'} size={10} />
        Run
      </button>
    </div>
  );
}
