import Color from 'color';
import React, { useState } from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import AnnotationInfo from '@webapp/pages/continuous/contextMenu/AnnotationInfo';
import useTimeZone from '@webapp/hooks/timeZone.hook';
import { faCommentDots } from '@fortawesome/free-regular-svg-icons/faCommentDots';

import styles from './styles.module.scss';

interface IAnnotationMarkProps {
  type: 'message';
  color: Color;
  value: {
    content: string;
    timestamp: number;
  };
}

const getIcon = (type: IAnnotationMarkProps['type']) => {
  switch (type) {
    case 'message':
    default:
      return faCommentDots;
  }
};

const AnnotationMark = ({ type, color, value }: IAnnotationMarkProps) => {
  const { offset } = useTimeZone();
  const [visible, toggle] = useState(false);
  const [target, setTarget] = useState<Element>();
  const [hovered, setHovered] = useState(false);

  const onClick = (e: React.MouseEvent<HTMLDivElement, MouseEvent>) => {
    e.stopPropagation();
    setTarget(e.target as Element);
    toggle((prev) => !prev);
  };

  return (
    <>
      <div
        data-testid="annotation_mark_wrapper"
        onClick={onClick}
        style={{ background: hovered ? color.darken(0.2).hex() : color.hex() }}
        className={styles.wrapper}
        role="none"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        <FontAwesomeIcon className={styles.icon} icon={getIcon(type)} />
      </div>
      {target ? (
        <AnnotationInfo
          popoverAnchorPoint={{ x: 0, y: 27 }}
          container={target}
          value={value}
          timezone={offset === 0 ? 'utc' : 'browser'}
          timestamp={value.timestamp}
          isOpen={visible}
          setIsOpen={toggle}
          popoverClassname={styles.form}
        />
      ) : null}
    </>
  );
};

export default AnnotationMark;
