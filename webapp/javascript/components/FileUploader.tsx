/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDispatch } from 'react-redux';
import { useDropzone } from 'react-dropzone';
import { faTrash } from '@fortawesome/free-solid-svg-icons/faTrash';
import Button from '@ui/Button';
import { addNotification } from '../redux/reducers/notifications';
import styles from './FileUploader.module.scss';

interface Props {
  file: File;
  setFile: (file: File, flamebearer: Record<string, unknown>) => void;
  className?: string;
}
export default function FileUploader({ file, setFile, className }: Props) {
  const dispatch = useDispatch();

  const onDrop = useCallback((acceptedFiles) => {
    if (acceptedFiles.length > 1) {
      throw new Error('Only a single file at a time is accepted.');
    }

    console.log(acceptedFiles);
    acceptedFiles.forEach((file) => {
      const reader = new FileReader();

      reader.onabort = () => console.log('file reading was aborted');
      reader.onerror = () => console.log('file reading has failed');
      reader.onload = () => {
        const binaryStr = reader.result;

        if (typeof binaryStr === 'string') {
          throw new Error('Expecting file in binary format but got a string');
        }

        try {
          // ArrayBuffer -> JSON
          const s = JSON.parse(
            String.fromCharCode.apply(null, new Uint8Array(binaryStr))
          );
          // Only check for flamebearer fields, the rest of the file format is checked on decoding.
          const fields = ['names', 'levels', 'numTicks', 'maxSelf'];
          fields.forEach((field) => {
            if (!(field in s.flamebearer))
              throw new Error(
                `Unable to parse uploaded file: field ${field} missing`
              );
          });
          setFile(file, s);
        } catch (e) {
          dispatch(
            addNotification({
              message: e.message,
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
    //    accept: 'application/json',
  });

  const onRemove = () => {
    setFile(null, null);
  };

  return (
    <section className={`${styles.container} ${className}`}>
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
          Currently analyzing file {file.name}
          &nbsp;
          <Button icon={faTrash} onClick={onRemove}>
            Remove
          </Button>
        </aside>
      )}
    </section>
  );
}
