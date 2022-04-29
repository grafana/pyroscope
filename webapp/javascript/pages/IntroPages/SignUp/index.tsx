import React, { useState } from 'react';
import cx from 'classnames';
import { Link } from 'react-router-dom';
import InputField from '@webapp/ui/InputField';
import StatusMessage from '@webapp/ui/StatusMessage';
import { useAppDispatch } from '@webapp/redux/hooks';
import { signUp, logIn } from '@webapp/services/users';
import { loadCurrentUser } from '@webapp/redux/reducers/user';
import useNavigateUserIntroPages from '@webapp/hooks/navigateUserIntroPages.hook';
import { isSignupEnabled } from '@webapp/util/features';
import inputStyles from '../InputGroup.module.css';
import styles from '../IntroPages.module.css';
import Divider from '../Divider';
import { PAGES } from '../../constants';

function SignUpPage() {
  const dispatch = useAppDispatch();
  const [form, setForm] = useState({
    errors: [],
  });

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const formData = event.target as typeof event.target & {
        username: { value: string };
        password: { value: string };
        fullName: { value: string };
        email: { value: string };
      };
      const username = formData.username.value;
      const password = formData.password.value;
      const fullName = formData.fullName.value;
      const email = formData.email.value;

      const res = await signUp({ username, password, fullName, email });
      if (res.isOk) {
        await logIn({ username, password });
        dispatch(loadCurrentUser());
        return;
      }

      throw res.error;
    } catch (e: any) {
      setForm({ ...form, errors: e.errors || [e.message] });
    }
  }

  useNavigateUserIntroPages();

  return (
    <div className={styles.loginWrapper}>
      <form className={styles.form} onSubmit={handleSubmit}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>Welcome to Pyroscope</h1>
          {isSignupEnabled ? (
            <h3>Sign up</h3>
          ) : (
            <>
              <p>
                Sing up functionality in not enabled. To learn more, please
                refer to{' '}
                <a
                  className={styles.link}
                  href="https://pyroscope.io/docs/auth-internal/"
                  target="_blank"
                  rel="noreferrer"
                >
                  documentation
                </a>
                .
              </p>
            </>
          )}
        </div>
        {isSignupEnabled ? (
          <>
            <div>
              <StatusMessage type="error" message={form.errors?.join(', ')} />
              <InputField
                type="text"
                name="username"
                label="Username"
                placeholder="Username"
                className={inputStyles.inputGroup}
                required
              />
              <InputField
                type="email"
                name="email"
                label="Email"
                placeholder="Email"
                className={inputStyles.inputGroup}
                required
              />
              <InputField
                type="text"
                name="fullName"
                label="Full Name"
                placeholder="Full Name"
                className={inputStyles.inputGroup}
                required
              />
              <InputField
                type="password"
                name="password"
                label="Password"
                placeholder="Password"
                className={inputStyles.inputGroup}
                required
              />
            </div>
            <button className={styles.button} type="submit">
              Sign up
            </button>
          </>
        ) : null}
        <Divider />

        <Link to={PAGES.LOGIN} className={cx(styles.button, styles.buttonDark)}>
          Go back to main page
        </Link>
      </form>
    </div>
  );
}

export default SignUpPage;
