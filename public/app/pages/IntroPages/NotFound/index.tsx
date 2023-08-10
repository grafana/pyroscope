import React from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import styles from '../IntroPages.module.css';

function NotFoundPage() {
  return (
    <div className={styles.loginWrapper}>
      <div className={styles.form}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>This page does not exist</h1>
        </div>
        <Link to="/" className={cx(styles.button, styles.buttonDark)}>
          Go back to main page
        </Link>
      </div>
    </div>
  );
}

export default NotFoundPage;
