import React from 'react';
import ReactNotification, {
  store as libStore,
} from 'react-notifications-component';
import 'react-notifications-component/dist/theme.css';

export default function Notification() {
  return <ReactNotification />;
}

// TODO
export const store = libStore;
