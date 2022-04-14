import React, { useState, useEffect } from 'react';
import { Link, useHistory } from 'react-router-dom';
import cx from 'classnames';
import Icon from '@webapp/ui/Icon';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import InputField from '@webapp/ui/InputField';
import StatusMessage from '@webapp/ui/StatusMessage';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { logIn } from '@webapp/services/users';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@webapp/redux/reducers/user';
import {
  isGithubEnabled,
  isGitlabEnabled,
  isGoogleEnabled,
  isSignupEnabled,
} from '@webapp/util/features';
import { GitlabIcon, GoogleIcon } from '../Icons';
import Divider from '../Divider';
import inputStyles from '../InputGroup.module.css';
import styles from '../IntroPages.module.css';
import buttonStyles from './buttons.module.css';

function SignInPage() {
  const history = useHistory();
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const [form, setForm] = useState({
    username: '',
    password: '',
    errors: [],
  });

  const handleFormChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      const { username, password } = {
        ...form,
      };

      const res = await logIn({ username, password });
      if (res.isOk) {
        dispatch(loadCurrentUser());
        return;
      }

      throw res.error;
    } catch (e: any) {
      setForm({ ...form, errors: e.errors || [e.message] });
    }
  }

  useEffect(() => {
    if (currentUser) {
      history.push('/');
    }
  }, [currentUser]);

  return (
    <div className={styles.loginWrapper}>
      <form className={styles.form} onSubmit={handleSubmit}>
        <div className={styles.formHeader}>
          <div className={styles.logo} />
          <h1>Welcome to Pyroscope</h1>
          <h3>Log in to continue</h3>
        </div>
        <div>
          <StatusMessage type="error" message={form.errors?.join(', ')} />
          <InputField
            id="username"
            type="text"
            name="username"
            label="Username"
            placeholder="Username"
            className={inputStyles.inputGroup}
            value={form.username}
            onChange={handleFormChange}
            required
          />
          <InputField
            id="password"
            type="password"
            name="password"
            label="Password"
            placeholder="Password"
            className={inputStyles.inputGroup}
            value={form.password}
            onChange={handleFormChange}
            required
          />
        </div>
        <button
          className={cx(styles.button, 'sign-in-button')}
          data-testid="sign-in-button"
          type="submit"
        >
          Log in
        </button>
        <Divider />
        <div className={cx(buttonStyles.buttonContainer)}>
          {isGoogleEnabled && (
            <a
              href="./auth/google/login"
              className={cx(styles.button, buttonStyles.buttonGoogle)}
            >
              <GoogleIcon /> Sign in with Google
            </a>
          )}
          {isGithubEnabled && (
            <a
              href="./auth/github/login"
              className={cx(styles.button, buttonStyles.buttonGithub)}
            >
              <Icon icon={faGithub} /> Sign in with GitHub
            </a>
          )}
          {isGitlabEnabled && (
            <a
              href="./auth/gitlab/login"
              className={cx(styles.button, buttonStyles.buttonGitlab)}
            >
              <GitlabIcon /> Sign in with GitLab
            </a>
          )}

          {isSignupEnabled && (
            <Link to="/signup" className={cx(styles.button, styles.buttonDark)}>
              Sign up
            </Link>
          )}
        </div>
      </form>
    </div>
  );
}

export default SignInPage;
