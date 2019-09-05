package templates

const ALIAS_YML = `
---
- hosts: web:&{{ env }}
  connection: local
  gather_facts: false
  tasks:
    - file:
        path: "{{ trellis_alias_temp_dir }}"
        state: directory
        mode: '0755'
      with_dict: "{{ wordpress_sites }}"
      run_once: true
    - template:
        src: "{{ trellis_alias_j2 }}"
        dest: "{{ trellis_alias_temp_dir }}/{{ env }}.yml.part"
        mode: '0644'
      with_dict: "{{ wordpress_sites }}"
      run_once: true
`
