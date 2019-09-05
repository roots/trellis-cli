package templates

const ALIAS_YML_J2 = `
@{{ env }}:
  ssh: "{{ web_user }}@{{ ansible_host }}:{{ ansible_port | default('22') }}"
  path: "{{ project_root | default(www_root + '/' + item.key) | regex_replace('^~\/','') }}/{{ item.current_path | default('current') }}/web/wp"
`
