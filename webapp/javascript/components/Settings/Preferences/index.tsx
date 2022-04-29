/* eslint-disable prettier/prettier */
import React, { useState } from 'react';
import Button from '@webapp/ui/Button';
import { useAppDispatch } from '@webapp/redux/hooks';

import { editMe, withCurrentUser } from '@webapp/redux/reducers/user';
import { addNotification } from '@webapp/redux/reducers/notifications';

import StatusMessage from '@webapp/ui/StatusMessage';
import InputField from '@webapp/ui/InputField';

function Preferences(props: ShamefulAny) {
  const { currentUser } = props;
  const dispatch = useAppDispatch();
  const [errors, setErrors] = useState<string | undefined>();

  const handleFormSubmit = (evt: React.FormEvent<HTMLFormElement>) => {
    evt.preventDefault();
    setErrors(undefined);
    const formData = evt.target as typeof evt.target & {
      name: { value: string };
      email: { value: string };
      fullName: { value: string };
    };
    const data = {
      name: formData.name.value,
      email: formData.email.value,
      fullName: formData.fullName.value,
    };
    dispatch(editMe(data))
      .unwrap()
      .then(
        () => {
          dispatch(
            addNotification({
              type: 'success',
              title: 'Success',
              message: 'User has been successfully edited',
            })
          );
        },
        (e) => setErrors(e.error)
      );
    return false;
  };

  const isEditDisabled = !!(currentUser && currentUser.isExternal);

  if (!currentUser) return <></>;
  return (
    <>
      <h2>Edit profile</h2>
      <form onSubmit={handleFormSubmit}>
        <StatusMessage type="error" message={errors || ''} />
        <InputField
          label="Username"
          type="text"
          placeholder="username"
          defaultValue={currentUser.name}
          name="name"
          required
          autoComplete="username"
          disabled={isEditDisabled}
        />
        <InputField
          label="Full Name"
          type="text"
          placeholder="Full Name"
          name="fullName"
          defaultValue={currentUser.fullName}
        />
        <InputField
          label="Email"
          type="text"
          placeholder="email"
          defaultValue={currentUser.email}
          autoComplete="email"
          name="email"
        />
        <Button type="submit" kind="secondary">
          Save
        </Button>
      </form>
    </>
  );
}

export default withCurrentUser(Preferences);
