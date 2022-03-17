import React, { useState, useEffect } from 'react';
import { Link, useHistory } from 'react-router-dom';
import cx from 'classnames';
import Icon from '@ui/Icon';
import { faGithub } from '@fortawesome/free-brands-svg-icons/faGithub';
import InputField from '@ui/InputField';
import StatusMessage from '@ui/StatusMessage';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import { logIn } from '@pyroscope/services/users';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@pyroscope/redux/reducers/user';
import { GitlabIcon, GoogleIcon } from '../Icons';
import Divider from '../Divider';
import inputStyles from '../InputGroup.module.css';
import styles from '../IntroPages.module.css';
import buttonStyles from './buttons.module.css';

function SignInPage(props) {
  const history = useHistory();
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const [form, setForm] = useState({
    username: '',
    password: '',
    errors: [],
  });

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  async function handleSubmit(event) {
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
    } catch (e) {
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
        <button className={styles.button} type="submit">
          Log in
        </button>
        <Divider />
        <div className={cx(buttonStyles.buttonContainer)}>
          <Link
            to="./auth/google/login"
            className={cx(styles.button, buttonStyles.buttonGoogle)}
          >
            <GoogleIcon /> Sign in with Google
          </Link>

          <Link
            to="./auth/github/login"
            className={cx(styles.button, buttonStyles.buttonGithub)}
          >
            <Icon icon={faGithub} /> Sign in with GitHub
          </Link>

          <Link
            to="./auth/gitlab/login"
            className={cx(styles.button, buttonStyles.buttonGitlab)}
          >
            <GitlabIcon /> Sign in with GitLab
          </Link>

          <Link to="/signup" className={cx(styles.button, styles.buttonDark)}>
            Sign up
          </Link>
        </div>
      </form>
    </div>
  );
}

export default SignInPage;
