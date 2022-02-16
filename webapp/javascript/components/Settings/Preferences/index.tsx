/* eslint-disable prettier/prettier */
import React, { useState } from 'react';
import Button from '@ui/Button';
import { useAppDispatch } from '@pyroscope/redux/hooks';

import {
  changeMyPassword,
  editMe,
  withCurrentUser,
} from '@pyroscope/redux/reducers/user';
import { addNotification } from '@pyroscope/redux/reducers/notifications';

import StatusMessage from '@ui/StatusMessage';
import InputField from '@ui/InputField';

function Preferences(props) {
  const { currentUser } = props;
  const dispatch = useAppDispatch();

  const [form, setForm] = useState(currentUser);
  const handleFormSubmit = (evt) => {
    evt.preventDefault();

    dispatch(editMe(form))
      .unwrap()
      .then(() => {
        dispatch(
          addNotification({
            type: 'success',
            title: 'Success',
            message: 'User has been successfully edited',
          })
        );
      });
    return false;
  };

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const isEditDisabled = !!(currentUser && currentUser.isExternal);

  if (!currentUser) return <></>;
  return (
    <>
      <h2>Edit profile</h2>
      <form onSubmit={handleFormSubmit}>
        <StatusMessage type="error">{form.errors}</StatusMessage>
        <InputField
          label="Username"
          type="text"
          placeholder="username"
          value={form?.name}
          name="name"
          required
          disabled={isEditDisabled}
          onChange={handleFormChange}
        />
        <InputField
          label="Full Name"
          type="text"
          placeholder="Full Name"
          name="fullName"
          value={form?.fullName}
          required
          onChange={handleFormChange}
        />
        <InputField
          label="Email"
          type="text"
          placeholder="email"
          value={form?.email}
          required
          name="email"
          onChange={handleFormChange}
        />
        <Button type="submit" kind="secondary">
          Save
        </Button>
      </form>
    </>
  );
}

export default withCurrentUser(Preferences);
