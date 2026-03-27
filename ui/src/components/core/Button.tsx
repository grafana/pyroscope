import './Button.css';

export function Button({
  children,
  onClick,
  active = false,
  title,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  active?: boolean;
  title?: string;
}) {
  return (
    <button
      title={title}
      onClick={onClick}
      data-active={active}
      className="btn"
    >
      {children}
    </button>
  );
}
