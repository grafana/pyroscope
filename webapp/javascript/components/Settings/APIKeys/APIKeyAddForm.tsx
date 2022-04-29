import React, { useState } from 'react';
import Button from '@webapp/ui/Button';
import InputField from '@webapp/ui/InputField';
import { CopyToClipboard } from 'react-copy-to-clipboard';
import { faCopy } from '@fortawesome/free-solid-svg-icons/faCopy';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { createAPIKey } from '@webapp/redux/reducers/settings';
import { useAppDispatch } from '@webapp/redux/hooks';
import { type APIKey } from '@webapp/models/apikeys';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import StatusMessage from '@webapp/ui/StatusMessage';
import { addNotification } from '@webapp/redux/reducers/notifications';
import styles from './APIKeyForm.module.css';

// Extend the API key, but add form validation errors and ttlSeconds

declare type Role = 'ReadOnly' | 'Admin';
export interface APIKeyAddProps extends Omit<APIKey, 'createdAt' | 'id'> {
  errors?: string[];
  ttlSeconds?: number;
}

function APIKeyAddForm() {
  const [form, setForm] = useState<APIKeyAddProps>({
    errors: [],
    name: '',
    role: 'ReadOnly',
    ttlSeconds: 360000,
  });
  const [key, setKey] = useState<string | undefined>();
  const dispatch = useAppDispatch();

  const handleRoleChange = (value: Role) => {
    setForm({ ...form, role: value });
  };

  const handleFormSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const formData = event.target as typeof event.target & {
      name: { value: string };
      ttlSeconds: { value: string };
    };
    const data = {
      name: formData.name.value,
      role: form.role,
      ttlSeconds: Number(formData.ttlSeconds.value),
    };
    dispatch(createAPIKey(data))
      .unwrap()
      .then(
        (k: APIKey): void => {
          setKey(k.key);
        },
        (e) => {
          setForm({ ...form, errors: e.errors });
        }
      );
  };

  const handleKeyCopy = () => {
    dispatch(
      addNotification({
        type: 'success',
        title: 'Success',
        message: 'Key has been copied',
      })
    );
  };

  return (
    <>
      <h2>Add API Key</h2>

      <form onSubmit={handleFormSubmit}>
        {key ? (
          <div>
            <StatusMessage
              type="success"
              message="Key has been successfully added. Click the button below to copy 
              it."
            />
            <div>
              <CopyToClipboard text={key} onCopy={handleKeyCopy}>
                <Button icon={faCopy} className={styles.keyOutput}>
                  {key}
                </Button>
              </CopyToClipboard>
            </div>
          </div>
        ) : (
          <>
            <InputField
              label="Name"
              placeholder="Name"
              id="keyName"
              type="text"
              name="name"
              required
            />
            <div>
              <h4>Role</h4>
              <Dropdown
                onItemClick={(i) => handleRoleChange(i.value)}
                value={form.role}
                label="Role"
              >
                <MenuItem value="Admin">Admin</MenuItem>
                <MenuItem value="ReadOnly">ReadOnly</MenuItem>
                <MenuItem value="Agent">Agent</MenuItem>
              </Dropdown>
            </div>
            <InputField
              label="Valid for (seconds):"
              id="keyTTL"
              name="ttlSeconds"
            />
            <div>
              <Button icon={faCheck} type="submit" kind="secondary">
                Add API Key
              </Button>
            </div>
          </>
        )}
      </form>
    </>
  );
}

export default APIKeyAddForm;
