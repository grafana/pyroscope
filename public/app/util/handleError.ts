import type { RequestError } from '@pyroscope/services/base';
import { ZodError } from 'zod';
import { addNotification } from '@pyroscope/redux/reducers/notifications';
import { useAppDispatch } from '@pyroscope/redux/hooks';

/**
 * handleError handles service errors
 */
export default async function handleError(
  dispatch: ReturnType<typeof useAppDispatch>,
  message: string,
  error: ZodError | RequestError
): Promise<void> {
  // We log the error in case a tech-savy user wants to debug themselves
  console.error(error);

  let errorMessage;
  if ('message' in error) {
    errorMessage = error.message;
  }

  // a ZodError means its format is not what we expect
  if (error instanceof ZodError) {
    errorMessage = 'response not in the expected format';
  }

  // display a notification
  dispatch(
    addNotification({
      title: 'Error',
      message: [message, errorMessage].join('\n'),
      type: 'danger',
    })
  );
}
