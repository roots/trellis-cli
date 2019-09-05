package templates

const ALIAS_COPY_YML = `
---
- hosts: web:&{{ env }}
  connection: local
  gather_facts: false
  tasks:
    - copy:
        src: "{{ trellis_alias_combined }}"
        dest: "{{ item.value.local_path }}/wp-cli.trellis-alias.yml"
        mode: '0644'
        force: yes
        decrypt: no
      with_dict: "{{ wordpress_sites }}"
      run_once: true
`
