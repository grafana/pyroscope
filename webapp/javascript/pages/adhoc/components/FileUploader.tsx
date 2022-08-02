/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import { Maybe } from '@webapp/util/fp';
import type { DropzoneOptions } from 'react-dropzone';

// Note: I wanted to use https://fontawesome.com/v6.0/icons/arrow-up-from-bracket?s=solid
// but it is in fontawesome v6 which is in beta and not released yet.
import { faArrowAltCircleUp } from '@fortawesome/free-regular-svg-icons/faArrowAltCircleUp';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import styles from './FileUploader.module.scss';

interface Props {
  filename: Maybe<string>;
  setFile: (file: File) => void;
  className?: string;
}
export default function FileUploader({ filename, setFile, className }: Props) {
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
        {filename.isJust ? (
          <div className={styles.subHeadingContainer}>
            <div className={styles.subHeading}>
              To analyze another file, drag and drop pprof, json, or collapsed
              files here or click to select a file
            </div>
            <div className={styles.headerMain}> {filename.value} </div>
          </div>
        ) : (
          <div>
            <p className={styles.headerMain}>
              Drag and drop pprof, json, or collapsed files here
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
