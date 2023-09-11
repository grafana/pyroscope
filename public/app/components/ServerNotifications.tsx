import { useEffect } from 'react';
import { useDispatch } from 'react-redux';
import { addNotification } from '@pyroscope/redux/reducers/notifications';

export default function ServerNotifications() {
  const dispatch = useDispatch();

  useEffect(() => {
    // the server is supposed to add this
    // to the index.html
    const { notificationText } = window as ShamefulAny;

    if (notificationText) {
      // TODO
      // distinguish between notification types?
      dispatch(
        addNotification({
          message: notificationText,
          type: 'danger',
          dismiss: {
            duration: 0,
            showIcon: true,
          },
        })
      );
    }
  }, [dispatch]);

  return null;
}
