import React, {
  useLayoutEffect,
  SetStateAction,
  Dispatch,
  ReactNode,
  useRef,
  useState,
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
      const popoverWidth = popoverRef.current.clientWidth;
      const windowWidth = window.innerWidth;
      const anchorPointX = anchorPoint.x;
      const threshold = 30;

      if (anchorPointX + popoverWidth + threshold >= windowWidth) {
        setPopoverPosition({
          left: `${anchorPoint.x - popoverWidth}px`,
          top: `${anchorPoint.y}px`,
          position: 'absolute' as const,
        });
      } else {
        // position to the right
        setPopoverPosition({
          left: `${anchorPoint.x}px`,
          top: `${anchorPoint.y}px`,
          position: 'absolute' as const,
        });
      }
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
