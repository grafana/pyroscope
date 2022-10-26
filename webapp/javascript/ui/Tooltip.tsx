/* eslint-disable 
jsx-a11y/click-events-have-key-events, 
jsx-a11y/no-noninteractive-element-interactions, 
css-modules/no-unused-class 
*/
import React, { useRef } from 'react';
import PopperUnstyled from '@mui/base/PopperUnstyled';
import MuiTooltip from '@mui/material/Tooltip';
import styles from './Tooltip.module.scss';

interface TooltipProps {
  title: string;
  visible: boolean;
  className?: string;
  placement: 'top' | 'left' | 'bottom' | 'right';
}

const Tooltip = ({ title, visible, className, placement }: TooltipProps) => {
  return (
    <div
      onClick={(e) => e.stopPropagation()}
      className={`${styles.tooltip} ${visible ? styles.visible : ''} ${
        styles?.[placement]
      } ${className || ''} `}
      role="tooltip"
    >
      {title}
    </div>
  );
};

//interface Tooltip2Props {
//  title: string;
//  placement: 'top' | 'left' | 'bottom' | 'right';
//  children: JSX.Element;
//}

const Tooltip2 = MuiTooltip;
// onMouseEnter={() => setRolesTooltipVisibility(true)}
//                onMouseLeave={() => setRolesTooltipVisibility(false)}
//function Tooltip2({ title, placement, children }: Tooltip2Props) {
//  const id = 'simple-popper';
//  const childRef = useRef();
//  const [childNode, setChildNode] = React.useState<HTMLElement>();
//
//  const cloned = React.cloneElement(children, {
//    ...children.props,
//    ref: childRef,
//  });
//
//  return (
//    <>
//      {cloned}
//      <PopperUnstyled
//        id={id}
//        open={childNode ? open : false}
//        anchorEl={childRef.current}
//      >
//        {title}
//      </PopperUnstyled>
//    </>
//  );
//}
//
export default Tooltip;
export { Tooltip2 };
