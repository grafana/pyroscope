/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDispatch } from 'react-redux';
import { useDropzone } from 'react-dropzone';
import { faTrash } from '@fortawesome/free-solid-svg-icons/faTrash';
import Button from '@ui/Button';
import { addNotification } from '../redux/reducers/notifications';
import styles from './FileUploader.module.scss';
import { deltaDiffWrapper } from '../util/flamebearer';

interface Props {
  onUpload: (s: string) => void;
  file: File;
  setFile: (file: File, flamebearer: Record<string, unknown>) => void;
}
export default function FileUploader({ file, setFile }: Props) {
  const dispatch = useDispatch();

  const onDrop = useCallback((acceptedFiles) => {
    if (acceptedFiles.length > 1) {
      throw new Error('Only a single file at a time is accepted.');
    }

    acceptedFiles.forEach((file) => {
      const reader = new FileReader();

      reader.onabort = () => console.log('file reading was aborted');
      reader.onerror = () => console.log('file reading has failed');
      reader.onload = () => {
        const binaryStr = reader.result;

        try {
          // ArrayBuffer -> JSON
          const s = JSON.parse(
            String.fromCharCode.apply(null, new Uint8Array(binaryStr))
          );
          const { flamebearer } = s;
          // TODO(abeaumont): Use versioned format checking and notifications system.
          const fields = [
            'names',
            'levels',
            'numTicks',
            'maxSelf',
            'format',
            'spyName',
            'format',
            'sampleRate',
            'units',
          ];
          fields.forEach((field) => {
            if (!(field in flamebearer))
              throw new Error(
                `Unable to parse uploaded file: field ${field} missing`
              );
          });
          const calculatedLevels = deltaDiffWrapper(
            flamebearer.format,
            flamebearer.levels
          );

          flamebearer.levels = calculatedLevels;
          setFile(file, flamebearer);
        } catch (e) {
          alert(e);
          dispatch(
            addNotification({
              message: e,
              type: 'danger',
              dismiss: {
                duration: 0,
                showIcon: true,
              },
            })
          );
        }
      };
      reader.readAsArrayBuffer(file);
    });
  }, []);
  const { getRootProps, getInputProps } = useDropzone({
    multiple: false,
    onDrop,
    accept: 'application/json',
  });

  const onRemove = () => {
    setFile(null, null);
  };

  return (
    <section className={styles.container}>
      <div {...getRootProps()}>
        <input {...getInputProps()} />
        {file ? (
          <p>
            To analyze another file, drag and drop pyroscope JSON files here or
            click to select a file
          </p>
        ) : (
          <p>
            Drag and drop pyroscope JSON files here, or click to select a file
          </p>
        )}
      </div>
      {file && (
        <aside>
          Currently analyzing file {file.path}
          &nbsp;
          <Button icon={faTrash} onClick={onRemove}>
            Remove
          </Button>
        </aside>
      )}
    </section>
  );
}
