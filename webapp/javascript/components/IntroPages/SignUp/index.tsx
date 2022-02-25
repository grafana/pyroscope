import React from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import InputField from '@ui/InputField';
import Divider from '../Divider';
import styles from '../IntroPages.module.css';
import inputStyles from '../InputGroup.module.css';

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
            styling={inputStyles.inputGroup}
          />
          <InputField
            label="Email"
            placeholder="Email"
            styling={inputStyles.inputGroup}
          />
          <InputField
            label="Full Name"
            placeholder="Full Name"
            styling={inputStyles.inputGroup}
          />
          <InputField
            label="Password"
            placeholder="Password"
            styling={inputStyles.inputGroup}
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
