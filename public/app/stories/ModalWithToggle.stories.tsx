/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import ModalWithToggle from '@pyroscope/ui/Modals/ModalWithToggle';
// import Button from '@pyroscope/ui/Button';
import { ComponentMeta } from '@storybook/react';
import '../sass/profile.scss';

export default {
  title: 'Components/ModalWithToggle',
  component: ModalWithToggle,
} as ComponentMeta<typeof ModalWithToggle>;

export const Bacis = () => {
  const [isOpen, setOpenStatus] = useState(false);

  const handleOutsideClick = () => setOpenStatus(false);
  const props = {
    isModalOpen: isOpen,
    setModalOpenStatus: setOpenStatus,
    handleOutsideClick,
    toggleText: 'toggle text',
    headerEl: 'header element',
    leftSideEl: (
      <ul>
        <li>1</li>
        <li>2</li>
      </ul>
    ),
    rightSideEl: (
      <ul>
        <li>3</li>
        <li>4</li>
      </ul>
    ),
    footerEl: 'footer element or string',
  };

  return (
    <div style={{ paddingLeft: 400 }}>
      <ModalWithToggle {...props} />
    </div>
  );
};

export const WithHeaderAndFooterEl = () => {
  const [isOpen, setOpenStatus] = useState(false);

  const handleOutsideClick = () => setOpenStatus(false);
  const props = {
    isModalOpen: isOpen,
    setModalOpenStatus: setOpenStatus,
    handleOutsideClick,
    toggleText: 'toggle text',
    headerEl: (
      <>
        <h3>modal title</h3>
        <input type="text" />
      </>
    ),
    leftSideEl: (
      <ul>
        <li>1</li>
        <li>2</li>
      </ul>
    ),
    rightSideEl: (
      <ul>
        <li>3</li>
        <li>4</li>
      </ul>
    ),
    footerEl: <button>button</button>,
  };

  return (
    <div style={{ paddingLeft: 400 }}>
      <ModalWithToggle {...props} />
    </div>
  );
};
