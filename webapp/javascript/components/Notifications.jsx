import React, { useState, useEffect } from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import { withShortcut } from 'react-keybind';
import Modal from 'react-modal';
import clsx from 'clsx';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faExclamationTriangle } from '@fortawesome/free-solid-svg-icons/faExclamationTriangle';

function Notifications(props) {
  const { notificationText } = window;

  const [hidden, setHidden] = useState(notificationText.length === 0);

  return (
    <div className={clsx('notifications', { hidden })}>
      <div className="notifications-container">
        <div className="notification-icon">
          <FontAwesomeIcon icon={faExclamationTriangle} />
        </div>
        <div className="notification-body">
          <ul className="notification-list">
            {notificationText.map((notification) => (
              <li>{notification}</li>
            ))}
          </ul>
        </div>
        <div
          className="notification-close-btn"
          onClick={function () {
            setHidden(true);
          }}
        />
      </div>
    </div>
  );
}

export default connect((x) => x, {})(Notifications);
