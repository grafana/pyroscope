type ServiceSelectorProps = {
  name: string;
};

export function ServiceSelector(props: ServiceSelectorProps) {
  const { name } = props;

  return (
    <>{name}</>
  );
}
