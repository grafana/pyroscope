import './Dropdown.css';

export function DropdownItem({
  children,
  onClick,
  selected,
  mono,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  selected?: boolean;
  mono?: boolean;
}) {
  return (
    <div
      onClick={onClick}
      data-selected={selected}
      data-mono={mono}
      className="dropdown-item"
    >
      {children}
    </div>
  );
}

export function DropdownSection({ label }: { label: string }) {
  return <div className="dropdown-section">{label}</div>;
}
