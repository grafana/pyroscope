import React, { useState } from 'react';
import Button from '@ui/Button';
import { CopyToClipboard } from 'react-copy-to-clipboard';
import { faCopy, faCheck } from '@fortawesome/free-solid-svg-icons';
import { createAPIKey } from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { type APIKey } from '@models/apikeys';
import Dropdown, { MenuItem } from '@ui/Dropdown';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import styles from './APIKeyForm.module.css';

export type APIKeyAddProps = APIKey;

function APIKeyAddForm() {
  const [form, setForm]: [APIKeyAddProps, (value) => void] = useState({
    errors: [],
    name: '',
    role: 'ReadOnly',
    ttlSeconds: 360000,
  });
  const [key, setKey] = useState(undefined);
  const dispatch = useAppDispatch();

  const handleFormChange = (event) => {
    const { name } = event.target;
    const { value } = event.target;
    setForm({ ...form, [name]: value });
  };

  const handleRoleChange = (value) => {
    setForm({ ...form, role: value });
  };

  const handleFormSubmit = (event) => {
    event.preventDefault();
    const data = {
      name: form.name,
      role: form.role,
      ttlSeconds: Number(form.ttlSeconds),
    };
    dispatch(createAPIKey(data))
      .unwrap()
      .then(
        (k) => {
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

      <div>{form.errors.join(', ')}</div>
      <form onSubmit={handleFormSubmit}>
        {key ? (
          <div>
            <div className={styles.success}>
              Key has been successfully added. Click the button below to copy it
              to clipboard
            </div>
            <div>
              <CopyToClipboard text={key} onCopy={handleKeyCopy}>
                <Button icon={faCopy} className={styles.keyOutput} />
              </CopyToClipboard>
            </div>
          </div>
        ) : (
          <div className={styles.addForm}>
            <div>
              <h4>Name</h4>
              <input
                id="keyName"
                type="text"
                name="name"
                value={form.name}
                onChange={handleFormChange}
              />
            </div>
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
            <div>
              <h4>Valid For (seconds):</h4>
              <input
                id="keyTTL"
                name="ttlSeconds"
                value={form.ttlSeconds}
                onChange={handleFormChange}
              />
            </div>
            <div>
              <Button icon={faCheck} type="submit" kind="secondary">
                Add API Key
              </Button>
            </div>
          </div>
        )}
      </form>
    </>
  );
}

export default APIKeyAddForm;
