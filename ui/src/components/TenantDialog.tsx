import { useRef } from 'react';
import { Button } from '@components/core/Button';
import './TenantDialog.css';

export function TenantDialog({
  currentTenantID,
  onSaved,
}: {
  currentTenantID?: string;
  onSaved: (id: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const id = inputRef.current?.value.trim();
    if (id) onSaved(id);
  };

  return (
    <div className="tenant-backdrop">
      <div className="tenant-dialog">
        <h2 className="tenant-dialog-title">Enter a Tenant ID</h2>
        <p className="tenant-dialog-body">
          Your Pyroscope instance has been detected as multitenant. Please enter
          a Tenant ID to continue.
        </p>
        <p className="tenant-dialog-hint">
          If you migrated from a single-tenant setup, your data can be found
          under the tenant ID <code>anonymous</code>.
        </p>
        <form className="tenant-dialog-form" onSubmit={handleSubmit}>
          <input
            ref={inputRef}
            className="tenant-dialog-input"
            type="text"
            defaultValue={currentTenantID}
            placeholder="e.g. anonymous"
            required
            autoFocus
          />
          <Button>Submit</Button>
        </form>
      </div>
    </div>
  );
}
