import React, { useEffect, useState } from 'react';
import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import TextField from '@webapp/ui/Form/TextField';
import {
  Dialog,
  DialogBody,
  DialogFooter,
  DialogHeader,
} from '@webapp/ui/Dialog';
import Button from '@webapp/ui/Button';
import {
  checkTenancyIsRequired,
  selectTenancy,
  actions,
  selectTenantID,
} from '@phlare/redux/reducers/tenant';

export function TenantWall({ children }: { children: React.ReactNode }) {
  const dispatch = useAppDispatch();
  const tenancy = useAppSelector(selectTenancy);
  const currentTenant = useAppSelector(selectTenantID);

  useEffect(() => {
    void dispatch(checkTenancyIsRequired());
  }, [dispatch]);

  // Don't rerender all the children when this component changes
  // For example, when user wants to change the tenant ID
  const memoedChildren = React.useMemo(() => children, [children]);

  switch (tenancy) {
    case 'unknown':
    case 'loading': {
      return <></>;
    }
    case 'wants_to_change': {
      return (
        <>
          <SelectTenantIDDialog
            currentTenantID={currentTenant}
            onSaved={(tenantID) => {
              void dispatch(actions.setTenantID(tenantID));
            }}
          />
          {memoedChildren}
        </>
      );
    }
    case 'needs_tenant_id': {
      return (
        <SelectTenantIDDialog
          currentTenantID={currentTenant}
          onSaved={(tenantID) => {
            void dispatch(actions.setTenantID(tenantID));
          }}
        />
      );
    }
    case 'multi_tenant':
    case 'single_tenant': {
      return <>{memoedChildren}</>;
    }
  }
}

function SelectTenantIDDialog({
  currentTenantID,
  onSaved,
}: {
  currentTenantID?: string;
  onSaved: (tenantID: string) => void;
}) {
  const [isDialogOpen] = useState(true);
  const handleFormSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();

    const target = e.target as typeof e.target & {
      tenantID: { value: string };
    };

    onSaved(target.tenantID.value);
  };

  return (
    <>
      <Dialog open={isDialogOpen} aria-labelledby="dialog-header">
        <>
          <DialogHeader>
            <h3 id="dialog-header">Enter a Tenant ID</h3>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              void handleFormSubmit(e);
            }}
          >
            <DialogBody>
              <>
                <p>
                  Your instance has been detected as a multitenant one. Please
                  enter a Tenant ID (You can change it at any time via the
                  sidebar).
                </p>
                <p>
                  Notice that if you migrated from a non-multitenant version,
                  data can be found under Tenant ID {'"'}anonymous{'"'}.
                </p>

                <TextField
                  defaultValue={currentTenantID}
                  label="Tenant ID"
                  required
                  id="tenantID"
                  type="text"
                  autoFocus
                />
              </>
            </DialogBody>
            <DialogFooter>
              <Button type="submit" kind="secondary">
                Submit
              </Button>
            </DialogFooter>
          </form>
        </>
      </Dialog>
    </>
  );
}
