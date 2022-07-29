/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import type { DropzoneOptions } from 'react-dropzone';
import { faTrash } from '@fortawesome/free-solid-svg-icons/faTrash';

// Note: I wanted to use https://fontawesome.com/v6.0/icons/arrow-up-from-bracket?s=solid
// but it is in fontawesome v6 which is in beta and not released yet.
import { faArrowAltCircleUp } from '@fortawesome/free-regular-svg-icons/faArrowAltCircleUp';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Button from '@webapp/ui/Button';
import styles from './FileUploader.module.scss';

interface Props {
  file: File;
  setFile: (file: File) => void;
  removeFile: () => void;
  className?: string;
}
export default function FileUploader({
  file,
  setFile,
  removeFile,
  className,
}: Props) {
  type onDrop = Required<DropzoneOptions>['onDrop'];
  const onDrop = useCallback<onDrop>(
    (acceptedFiles) => {
      if (acceptedFiles.length > 1) {
        throw new Error('Only a single file at a time is accepted.');
      }

      acceptedFiles.forEach((f) => {
        setFile(f);
      });
    },
    [setFile]
  );

  const { getRootProps, getInputProps } = useDropzone({
    multiple: false,
    onDrop,
  });

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
              <Button icon={faTrash} onClick={removeFile}>
                Remove
              </Button>
            </div>
          </div>
        ) : (
          <div>
            <p className={styles.headerMain}>Drag and drop files here</p>
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
