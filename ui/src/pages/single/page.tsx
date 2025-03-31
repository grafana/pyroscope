import { Button, Stack } from '@grafana/ui';
import { ServiceSelector } from './ServiceSelector';

export function SinglePage() {
  return (
    <Stack direction="column">
      <ServiceSelector name="Single" />

      <Button>
        Click me
      </Button>
    </Stack>
  );
}
