import * as annotationsService from '@pyroscope/services/annotations';
import { addNotification } from '../notifications';
import { createAsyncThunk } from '../../async-thunk';

// TODO(eh-am): support different views
export const addAnnotation = createAsyncThunk(
  'continuous/addAnnotation',
  async (newAnnotation: annotationsService.NewAnnotation, thunkAPI) => {
    const res = await annotationsService.addAnnotation(newAnnotation);

    if (res.isOk) {
      return Promise.resolve({
        annotation: res.value,
      });
    }

    thunkAPI.dispatch(
      addNotification({
        type: 'danger',
        title: 'Failed to add annotation',
        message: res.error.message,
      })
    );

    return Promise.reject(res.error);
  }
);
