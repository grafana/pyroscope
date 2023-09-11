import React, { useState } from 'react';
import Button from '@pyroscope/ui/Button';
import InputField from '@pyroscope/ui/InputField';
import { TooltipInfoIcon } from '@pyroscope/ui/TooltipInfoIcon';
import { CopyToClipboard } from 'react-copy-to-clipboard';
import { faCopy } from '@fortawesome/free-solid-svg-icons/faCopy';
import { faCheck } from '@fortawesome/free-solid-svg-icons/faCheck';
import { createAPIKey } from '@pyroscope/redux/reducers/settings';
import { useAppDispatch } from '@pyroscope/redux/hooks';
import { type APIKey } from '@pyroscope/models/apikeys';
import Dropdown, { MenuItem } from '@pyroscope/ui/Dropdown';
import { Tooltip } from '@pyroscope/ui/Tooltip';
import StatusMessage from '@pyroscope/ui/StatusMessage';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
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
  const [key, setKey] = useState<string>();
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
        (k) => {
          if (k.key) {
            setKey(k.key);
          }
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
              <h4>
                Role
                <Tooltip
                  title={
                    <div className={styles.rolesTooltip}>
                      {[
                        `Admin: Manage users/API keys`,
                        `ReadOnly: Used only for visualizations (i.e. Grafana datasource, embedding in a UI)`,
                        `Agent: Used for "authentication token" in code when auth is enabled`,
                      ].map((r) => (
                        <div key={r}>{r}</div>
                      ))}
                    </div>
                  }
                >
                  <TooltipInfoIcon />
                </Tooltip>
              </h4>
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
              type="number"
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
