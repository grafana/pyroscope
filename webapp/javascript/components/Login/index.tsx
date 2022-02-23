import React from 'react';
import cx from 'classnames';
import styles from './Login.module.css';

function LoginPage(props) {
  return (
    <div className={cx(styles.loginWrapper)}>
      <form className={styles.form} />
    </div>
  );
}

export default LoginPage;
