import React, {
  useRef,
  useState,
  useLayoutEffect,
  SetStateAction,
  Dispatch,
  ReactNode,
} from 'react';
import classnames from 'classnames';
import OutsideClickHandler from 'react-outside-click-handler';
import styles from './Popover.module.scss';

export interface PopoverProps {
  isModalOpen: boolean;
  setModalOpenStatus: Dispatch<SetStateAction<boolean>>;
  children: ReactNode;
  className?: string;

  /** where to position the popover on the page */
  anchorPoint: {
    x: number;
    y: number;
  };
}

export function Popover({
  isModalOpen,
  setModalOpenStatus,
  className,
  children,
  anchorPoint,
}: PopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const [popoverPosition, setPopoverPosition] = useState<React.CSSProperties>({
    display: 'hidden',
  });

  useLayoutEffect(() => {
    if (isModalOpen && popoverRef.current) {
      const pos = getPopoverPosition(
        popoverRef.current.clientWidth,
        window.innerWidth,
        anchorPoint
      );
      setPopoverPosition(pos);
    }
  }, [isModalOpen]);

  return (
    <OutsideClickHandler onOutsideClick={() => setModalOpenStatus(false)}>
      <div
        className={styles.container}
        style={popoverPosition}
        ref={popoverRef}
      >
        {isModalOpen && (
          <div className={classnames(styles.popover, className)}>
            {children}
          </div>
        )}
      </div>
    </OutsideClickHandler>
  );
}

function getPopoverPosition(
  popoverWidth: number,
  windowWidth: number,
  anchorPoint: PopoverProps['anchorPoint']
) {
  // Give some room between popover end and the window edge
  const marginToWindowEdge = 30;
  const defaultProps = {
    top: `${anchorPoint.y}px`,
    position: 'absolute' as const,
  };

  if (anchorPoint.x + popoverWidth + marginToWindowEdge >= windowWidth) {
    // position to the left
    return {
      ...defaultProps,
      left: `${anchorPoint.x - popoverWidth}px`,
    };
  }

  // position to the right
  return {
    ...defaultProps,
    left: `${anchorPoint.x}px`,
  };
}
interface PopoverMemberProps {
  children: ReactNode;
}

export function PopoverHeader({ children }: PopoverMemberProps) {
  return <div className={styles.header}>{children}</div>;
}

export function PopoverBody({ children }: PopoverMemberProps) {
  return <div className={styles.body}>{children}</div>;
}

export function PopoverFooter({ children }: PopoverMemberProps) {
  return <div className={styles.footer}>{children}</div>;
}
