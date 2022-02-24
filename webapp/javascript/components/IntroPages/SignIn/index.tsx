import React from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import InputField from '@ui/InputField';
import Divider from '../Divider';
import styles from '../IntroPages.module.css';

function SignInPage(props) {
  return (
    <div className={styles.loginWrapper}>
      <form className={styles.form}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>Welcome to Pyroscope</h1>
          <h3>Log in to continue</h3>
        </div>
        <div>
          <InputField
            label="Username"
            placeholder="Username"
            styling={styles.inputGroup}
          />
          <InputField
            label="Password"
            placeholder="Password"
            styling={styles.inputGroup}
          />
        </div>
        <button className={styles.button}>Log in</button>
        <Divider />

        <Link to="/signup" className={cx(styles.button, styles.buttonDark)}>
          Sign up
        </Link>
      </form>
    </div>
  );
}

export default SignInPage;
