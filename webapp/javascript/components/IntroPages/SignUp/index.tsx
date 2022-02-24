import React from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import InputField from '@ui/InputField';
import Divider from '../Divider';
import styles from '../IntroPages.module.css';

function SignUpPage(props) {
  return (
    <div className={styles.loginWrapper}>
      <form className={styles.form}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>Welcome to Pyroscope</h1>
          <h3>Sign up</h3>
        </div>
        <div>
          <InputField
            label="Username"
            placeholder="Username"
            styling={styles.inputGroup}
          />
          <InputField
            label="Email"
            placeholder="Email"
            styling={styles.inputGroup}
          />
          <InputField
            label="Full Name"
            placeholder="Full Name"
            styling={styles.inputGroup}
          />
          <InputField
            label="Password"
            placeholder="Password"
            styling={styles.inputGroup}
          />
        </div>
        <button className={styles.button}>Sign in</button>
        <Divider />

        <Link to="/signin" className={cx(styles.button, styles.buttonDark)}>
          Go back to main page
        </Link>
      </form>
    </div>
  );
}

export default SignUpPage;
