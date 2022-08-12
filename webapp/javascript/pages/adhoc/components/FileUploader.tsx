/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import Button from '@webapp/ui/Button';
import type { DropzoneOptions } from 'react-dropzone';
import { faFileUpload } from '@fortawesome/free-solid-svg-icons/faFileUpload';
import classNames from 'classnames/bind';
import UploadIcon from './UploadIcon';
import styles from './FileUploader.module.scss';

const cx = classNames.bind(styles);

interface Props {
  setFile: (file: File) => void;
  className?: string;
}
export default function FileUploader({ setFile: onUpload, className }: Props) {
  const [file, setFile] = React.useState<File>();
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

  const descriptionOrFilename = file
    ? file.name
    : 'Upload profile in pprof, json, or collapsed format';

  return (
    <>
      <section className={`${styles.container} ${className}`}>
        <div {...getRootProps()} className={styles.dragAndDropContainer}>
          <input {...getInputProps()} />
          <p className={styles.headerMain}>{descriptionOrFilename}</p>
          <div className={styles.iconContainer}>
            <UploadIcon />
          </div>
          <p className={styles.uploadBtnPreLabel}>
            Drag & Drop
            <span className={styles.uploadBtnPreLabel}>or</span>
          </p>
          <div className={styles.uploadBtnWrapper}>
            <Button
              kind="primary"
              className={cx({
                [styles.uploadButton]: true,
                [styles.disabled]: !!file,
              })}
              icon={faFileUpload}
              disabled={!!file}
            >
              Select a file
            </Button>
          </div>
        </div>
      </section>
      <div className={styles.uploadWrapper}>
        <Button
          kind="primary"
          disabled={!file}
          onClick={() => {
            if (file) {
              onUpload(file);
            }
          }}
        >
          Save
        </Button>
      </div>
    </>
  );
}
