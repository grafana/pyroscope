import React from 'react';

import { withCurrentUser } from '@pyroscope/redux/reducers/user';
import ChangePasswordForm from './ChangePasswordForm';

function Security(props: ShamefulAny) {
  const { currentUser } = props;

  if (!currentUser) {
    return <></>;
  }

  return (
    <>
      <ChangePasswordForm user={currentUser} />
    </>
  );
}

export default withCurrentUser(Security);
