import { DropdownItem } from '@components/core/Dropdown';
import './SuggestionList.css';

export function SuggestionList({
  items,
  highlightedIndex,
  onHover,
  onAccept,
}: {
  items: string[];
  highlightedIndex: number;
  onHover: (i: number) => void;
  onAccept: (item: string) => void;
}) {
  return (
    <div className="suggestion-list" role="listbox">
      {items.map((item, i) => (
        <div
          key={item}
          // Prevent the click from blurring the input — onMouseDown fires
          // before the input loses focus, so preventDefault here keeps the
          // caret in place and lets onAccept update both value and selection.
          onMouseDown={(e) => {
            e.preventDefault();
            onAccept(item);
          }}
          onMouseEnter={() => onHover(i)}
        >
          <DropdownItem selected={i === highlightedIndex} mono>
            <span className="suggestion-text">{item}</span>
          </DropdownItem>
        </div>
      ))}
    </div>
  );
}
