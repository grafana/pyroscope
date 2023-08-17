import React, { useState } from 'react';
import Button from '@pyroscope/ui/Button';
import InputField from '@pyroscope/ui/InputField';
import { useHistory } from 'react-router-dom';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { createUser } from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { passwordEncode, type User } from '@pyroscope/models/users';

export type UserAddProps = User & { password?: string };

function UserAddForm() {
  //  const [form, setForm]: [UserAddProps, (value: ShamefulAny) => void] =
  const [form, setForm] = useState({
    name: '',
    email: '',
    fullName: '',
    password: '',
  });
  const dispatch = useAppDispatch();
  const history = useHistory();

  const handleFormChange = (event: ShamefulAny) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const handleFormSubmit = (e: ShamefulAny) => {
    e.preventDefault();
    if (!form.password) {
      return;
    }

    const data = {
      ...form,
      role: 'ReadOnly',
      password: passwordEncode(form.password),
    };
    dispatch(createUser(data as ShamefulAny as User))
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
        <InputField
          label="Name"
          id="userAddName"
          name="name"
          value={form.name}
          onChange={handleFormChange}
          type="text"
        />
        <InputField
          label="Email"
          id="userAddEmail"
          name="email"
          value={form.email}
          onChange={handleFormChange}
          type="text"
        />
        <InputField
          label="Full name"
          id="userAddFullName"
          name="fullName"
          value={form.fullName}
          onChange={handleFormChange}
          type="text"
        />
        <InputField
          label="Password"
          id="userAddPassword"
          name="password"
          type="password"
          onChange={handleFormChange}
          value={form.password}
        />
        <div>
          <Button
            icon={faCheck}
            type="submit"
            data-testid="settings-useradd"
            kind="secondary"
          >
            Add user
          </Button>
        </div>
      </form>
    </>
  );
}

export default UserAddForm;
