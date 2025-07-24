import { Cascader, CascaderOption } from "@grafana/ui";

export function SinglePage() {
  const options: CascaderOption[] = [
    { label: 'option 1', value: '1' }
  ];

  return (
    <>
      <Cascader
        options={options}
        onSelect={(v) => console.log(v)}
      />
    </>
  );
}
