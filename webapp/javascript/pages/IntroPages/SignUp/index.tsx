import React, { useState, useEffect } from 'react';
import cx from 'classnames';
import { Link, useHistory } from 'react-router-dom';
import InputField from '@webapp/ui/InputField';
import StatusMessage from '@webapp/ui/StatusMessage';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import { signUp, logIn } from '@webapp/services/users';
import {
  loadCurrentUser,
  selectCurrentUser,
} from '@webapp/redux/reducers/user';
import inputStyles from '../InputGroup.module.css';
import styles from '../IntroPages.module.css';
import Divider from '../Divider';

function SignUpPage() {
  const history = useHistory();
  const dispatch = useAppDispatch();
  const currentUser = useAppSelector(selectCurrentUser);
  const [form, setForm] = useState({
    username: '',
    password: '',
    fullName: '',
    email: '',
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
      const { username, password, fullName, email } = {
        ...form,
      };

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

  // redirect when current user gets or is already set
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
          <h3>Sign up</h3>
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
            type="email"
            name="email"
            label="Email"
            placeholder="Email"
            className={inputStyles.inputGroup}
            value={form.email}
            onChange={handleFormChange}
            required
          />
          <InputField
            type="text"
            name="fullName"
            label="Full Name"
            placeholder="Full Name"
            className={inputStyles.inputGroup}
            value={form.fullName}
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
          Sign up
        </button>
        <Divider />

        <Link to="/login" className={cx(styles.button, styles.buttonDark)}>
          Go back to main page
        </Link>
      </form>
    </div>
  );
}

export default SignUpPage;
