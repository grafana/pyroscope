/* eslint-disable react/jsx-props-no-spreading */
import React, { useCallback, useState } from 'react';
import { useDropzone } from 'react-dropzone';
import Button from '@pyroscope/ui/Button';
import type { DropzoneOptions } from 'react-dropzone';
import { faFileUpload } from '@fortawesome/free-solid-svg-icons/faFileUpload';
import classNames from 'classnames/bind';
import Dropdown, { MenuItem } from '@pyroscope/ui/Dropdown';
import {
  SpyNameFirstClass,
  SpyNameFirstClassType,
} from '@pyroscope/legacy/models/spyName';
import { units as possibleUnits, UnitsType } from '@pyroscope/legacy/models';
import { humanizeSpyname, isJSONFile, humanizeUnits } from './humanize';
import UploadIcon from './UploadIcon';
import styles from './FileUploader.module.scss';

const cx = classNames.bind(styles);

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
  const [spyName, setSpyName] = useState<SpyNameFirstClassType>('gospy');
  const [units, setUnits] = useState<UnitsType>('samples');
  type OnDrop = Required<DropzoneOptions>['onDrop'];
  const onDrop = useCallback<OnDrop>(
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
            value={`Profile language: ${humanizeSpyname(spyName)}`}
            onItemClick={(e) => setSpyName(e.value)}
            label="Profile language"
          >
            {SpyNameFirstClass.map((name, index) => (
              <MenuItem
                className={cx({
                  [styles.activeDropdownItem]: spyName === name,
                })}
                key={String(index) + name}
                value={name}
              >
                {humanizeSpyname(name)}
              </MenuItem>
            ))}
          </Dropdown>
          <Dropdown
            value={`Profile units: ${humanizeUnits(units)}`}
            onItemClick={(e) => setUnits(e.value)}
            label="Profile units"
          >
            {possibleUnits.map((name, index) => (
              <MenuItem
                className={cx({
                  [styles.activeDropdownItem]: units === name,
                })}
                key={String(index) + name}
                value={name}
              >
                {humanizeUnits(name)}
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
