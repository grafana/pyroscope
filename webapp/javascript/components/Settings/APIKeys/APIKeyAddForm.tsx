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
export interface APIKeyAddProps extends APIKey {
  errors?: string[];
  ttlSeconds?: number;
}

function APIKeyAddForm() {
  //  const [form, setForm]: [APIKeyAddProps, (value) => void] = useState({
  const [form, setForm] = useState<ShamefulAny>({
    errors: [],
    name: '',
    role: 'ReadOnly',
    ttlSeconds: 360000,
  });
  const [key, setKey] = useState(undefined);
  const dispatch = useAppDispatch();

  const handleFormChange = (event: ShamefulAny) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const handleRoleChange = (value: ShamefulAny) => {
    setForm({ ...form, role: value });
  };

  const handleFormSubmit = (event: ShamefulAny) => {
    event.preventDefault();
    const data = {
      name: form.name,
      role: form.role,
      ttlSeconds: Number(form.ttlSeconds),
    };
    dispatch(createAPIKey(data))
      .unwrap()
      .then(
        (k: ShamefulAny) => {
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
              value={form.name}
              onChange={handleFormChange}
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
              value={form.ttlSeconds}
              onChange={handleFormChange}
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
