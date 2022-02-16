import React, { useState } from 'react';
import Button from '@ui/Button';
import { useHistory } from 'react-router-dom';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { createUser } from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { passwordEncode, type User } from '../../../models/users';
import styles from './UserForm.module.css';

export type UserAddProps = User & { password?: string };

function UserAddForm() {
  const [form, setForm]: [UserAddProps, (value) => void] = useState({
    name: '',
    email: '',
    fullName: '',
    password: '',
  });
  const dispatch = useAppDispatch();
  const history = useHistory();

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const handleFormSubmit = (e) => {
    e.preventDefault();
    const data = {
      ...form,
      role: 'ReadOnly',
      password: passwordEncode(form.password),
    };
    dispatch(createUser(data as User))
      .unwrap()
      .then(() => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'User added',
            message: `User has been successfully added`,
          })
        );
        history.push('/settings/users');
      });
  };

  return (
    <>
      <h2>Add User</h2>
      <form onSubmit={handleFormSubmit}>
        <div className={styles.userForm}>
          <div>
            <h4>Name</h4>
            <input
              id="userAddName"
              name="name"
              value={form.name}
              onChange={handleFormChange}
            />
          </div>
          <div>
            <h4>Email</h4>
            <input
              id="userAddEmail"
              name="email"
              value={form.email}
              onChange={handleFormChange}
            />
          </div>
          <div>
            <h4>Full Name</h4>
            <input
              id="userAddFullName"
              name="fullName"
              value={form.fullName}
              onChange={handleFormChange}
            />
          </div>
          <div>
            <h4>Password</h4>
            <input
              id="userAddPassword"
              name="password"
              type="password"
              onChange={handleFormChange}
            />
          </div>
          <div>
            <Button
              icon={faCheck}
              type="submit"
              kind="secondary"
              data-testid="settings-useradd"
            >
              Add user
            </Button>
          </div>
        </div>
      </form>
    </>
  );
}

export default UserAddForm;
