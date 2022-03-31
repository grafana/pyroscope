/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDispatch } from 'react-redux';
import { useDropzone } from 'react-dropzone';
import { faTrash } from '@fortawesome/free-solid-svg-icons/faTrash';

// Note: I wanted to use https://fontawesome.com/v6.0/icons/arrow-up-from-bracket?s=solid
// but it is in fontawesome v6 which is in beta and not released yet.
import { faArrowAltCircleUp } from '@fortawesome/free-regular-svg-icons/faArrowAltCircleUp';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from '@webapp/ui/Button';
import { addNotification } from '@webapp/redux/reducers/notifications';
import styles from './FileUploader.module.scss';

interface Props {
  file: File;
  setFile: (
    file: File | null,
    flamebearer: Record<string, unknown> | null
  ) => void;

  className?: string;
}
export default function FileUploader({ file, setFile, className }: Props) {
  const dispatch = useDispatch();

  const onDrop = useCallback((acceptedFiles) => {
    if (acceptedFiles.length > 1) {
      throw new Error('Only a single file at a time is accepted.');
    }

    acceptedFiles.forEach((file: ShamefulAny) => {
      const reader = new FileReader();

      reader.onabort = () => console.log('file reading was aborted');
      reader.onerror = () => console.log('file reading has failed');
      reader.onload = () => {
        const binaryStr = reader.result;

        if (typeof binaryStr === 'string') {
          throw new Error('Expecting file in binary format but got a string');
        }
        if (binaryStr === null) {
          throw new Error('Expecting file in binary format but got null');
        }

        try {
          // ArrayBuffer -> JSON
          const s = JSON.parse(
            String.fromCharCode.apply(
              null,
              new Uint8Array(binaryStr) as ShamefulAny
            )
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
        } catch (e: ShamefulAny) {
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
    accept: 'application/json',
  });

  const onRemove = () => {
    setFile(null, null);
  };

  return (
    <section className={`${styles.container} ${className}`}>
      <div {...getRootProps()} className={styles.dragAndDropContainer}>
        <input {...getInputProps()} />
        {file ? (
          <div className={styles.subHeadingContainer}>
            <div className={styles.subHeading}>
              To analyze another file, drag and drop pyroscope JSON files here
              or click to select a file
            </div>
            <div className={styles.headerMain}> {file.name} </div>
            <div className={styles.subHeading}>
              <Button icon={faTrash} onClick={onRemove}>
                Remove
              </Button>
            </div>
          </div>
        ) : (
          <div>
            <p className={styles.headerMain}>
              Drag and drop Flamegraph files here
            </p>
            <div className={styles.iconContainer}>
              <FontAwesomeIcon
                icon={faArrowAltCircleUp}
                className={styles.fileUploadIcon}
              />
            </div>
            <p className={styles.subHeading}>
              Or click to select a file from your device
            </p>
          </div>
        )}
      </div>
    </section>
  );
}
