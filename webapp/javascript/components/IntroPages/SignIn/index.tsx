import React, { useState, useEffect } from 'react';
import cx from 'classnames';
import { Link, useHistory } from 'react-router-dom';
import InputField from '@ui/InputField';
import StatusMessage from '@ui/StatusMessage';
import { useAppDispatch, useAppSelector } from '@pyroscope/redux/hooks';
import { logIn } from '@pyroscope/services/users';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@pyroscope/redux/reducers/user';
import inputStyles from '../InputGroup.module.css';
import styles from '../IntroPages.module.css';
import Divider from '../Divider';

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

      await logIn({ username, password });

      dispatch(loadCurrentUser());
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
            styling={inputStyles.inputGroup}
            value={form.username}
            onChange={handleFormChange}
            required
          />
          <InputField
            type="password"
            name="password"
            label="Password"
            placeholder="Password"
            styling={inputStyles.inputGroup}
            value={form.password}
            onChange={handleFormChange}
            required
          />
        </div>
        <button className={styles.button} type="submit">
          Log in
        </button>
        <Divider />

        <Link to="/signup" className={cx(styles.button, styles.buttonDark)}>
          Sign up
        </Link>
      </form>
    </div>
  );
}

export default SignInPage;
