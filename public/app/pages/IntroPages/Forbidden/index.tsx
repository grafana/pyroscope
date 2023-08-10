import React from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import styles from '../IntroPages.module.css';

function ForbiddenPage() {
  return (
    <div className={styles.loginWrapper}>
      <div className={styles.form}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>Authentication error</h1>
        </div>
        <Link to="/login" className={cx(styles.button, styles.buttonDark)}>
          Go back to login page
        </Link>
      </div>
    </div>
  );
}

export default ForbiddenPage;
