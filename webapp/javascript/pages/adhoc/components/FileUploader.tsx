/* eslint-disable react/jsx-props-no-spreading, jsx-a11y/role-supports-aria-props */
import React, { useCallback, useState } from 'react';
import { useDropzone } from 'react-dropzone';
import Button from '@webapp/ui/Button';
import type { DropzoneOptions } from 'react-dropzone';
import { faFileUpload } from '@fortawesome/free-solid-svg-icons/faFileUpload';
import classNames from 'classnames/bind';
import Dropdown, { MenuItem } from '@webapp/ui/Dropdown';
import UploadIcon from './UploadIcon';
import styles from './FileUploader.module.scss';

const cx = classNames.bind(styles);

const isJSONFile = (file: File) =>
  file.name.match(/\.json$/) ||
  file.type === 'application/json' ||
  file.type === 'text/json';

const SPYNAMES = {
  gospy: 'Go',
  pyspy: 'Python',
  phpspy: 'PHP',
  'pyroscope-rs': 'Rust',
  rbspy: 'Ruby',
  javaspy: 'Java',
  dotnetspy: '.NET',
  nodespy: 'NodeJS',
  ebpfspy: 'eBPF',
  other: 'Other',
};

const UNITS = {
  samples: 'Samples',
  objects: 'Objects',
  goroutines: 'Goroutines',
  bytes: 'Bytes',
  lock_samples: 'Lock Samples',
  lock_nanoseconds: 'Lock Nanoseconds',
  trace_samples: 'Trace Samples',
};

type UploadArgsType = {
  file: File;
  spyName?: string;
  units?: string;
};
interface Props {
  setFile: ({ file, spyName, units }: UploadArgsType) => void;
  className?: string;
}

export default function FileUploader({ setFile: onUpload, className }: Props) {
  const [file, setFile] = useState<File>();
  const [spyName, setSpyName] = useState<string>('gospy');
  const [units, setUnits] = useState('samples');
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

  const showLanguageAndUnits = (file && !isJSONFile(file)) || false;

  const { getRootProps, getInputProps } = useDropzone({
    multiple: false,
    onDrop,
  });

  const descriptionOrFilename = file
    ? file.name
    : 'Upload profile in pprof, json, or collapsed format';

  const onUploadClick = () => {
    if (file) {
      onUpload({
        file,
        spyName: showLanguageAndUnits ? spyName : undefined,
        units: showLanguageAndUnits ? units : undefined,
      });
    }
  };

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
      {showLanguageAndUnits && (
        <div className={styles.dropdowns}>
          <Dropdown
            value={`Profile language: ${
              SPYNAMES[spyName as keyof typeof SPYNAMES]
            }`}
            onItemClick={(e) => setSpyName(e.value as string)}
            label="Profile language"
          >
            {Object.keys(SPYNAMES).map((name, index) => (
              <MenuItem
                className={cx({
                  [styles.activeDropdownItem]: spyName === name,
                })}
                key={String(index) + name}
                value={name}
              >
                {SPYNAMES[name as keyof typeof SPYNAMES]}
              </MenuItem>
            ))}
          </Dropdown>
          <Dropdown
            value={`Profile units: ${UNITS[units as keyof typeof UNITS]}`}
            onItemClick={(e) => setUnits(e.value as string)}
            label="Profile units"
          >
            {Object.keys(UNITS).map((name, index) => (
              <MenuItem
                className={cx({
                  [styles.activeDropdownItem]: units === name,
                })}
                key={String(index) + name}
                value={name}
              >
                {UNITS[name as keyof typeof UNITS]}
              </MenuItem>
            ))}
          </Dropdown>
        </div>
      )}
      <div className={styles.uploadWrapper}>
        <Button kind="primary" disabled={!file} onClick={onUploadClick}>
          Save
        </Button>
      </div>
    </>
  );
}
