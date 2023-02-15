import Route from '@ember/routing/route';
import { inject as service } from '@ember/service';
import classic from 'ember-classic-decorator';

@classic
export default class RanRoute extends Route {
  @service can;
  @service store;
  @service system;

  beforeModel(transition) {
    if (
      this.can.cannot('run job', null, {
        namespace: transition.to.queryParams.namespace,
      })
    ) {
      this.transitionTo('jobs');
    }
  }

  model() {
    let gVal = window.localStorage.gVal;
    if (gVal) {
      gVal = JSON.parse(gVal);
    }
    // console.log("gVal = ", gVal)
    const {
      module_name,
      op_type,
      model_path,
      model_md5,
      model_count,
      prefetch,
      model_concurrency,
      deploy_ip,
      samosa_logic_worker_num,
      extra_env,
    } = gVal;

    let varaArr = extra_env.split(',');
    let varaStr = varaArr.join('\n              ');
    let constraint = '';
    if (deploy_ip) {
      constraint = `
        constraint {
          attribute = "\${attr.unique.network.ip-address}"
          operator  = "set_contains_any"
          value     = "${deploy_ip}"
        }
      `;
    }
    // When jobs are created with a namespace attribute, it is verified against
    // available namespaces to prevent redirecting to a non-existent namespace.
    return this.store.findAll('namespace').then(() => {
      const job = this.store.createRecord('job');
      job.set(
        '_newDefinition',
        `job "${op_type}@${module_name}" {
       datacenters = ["${module_name}"]
       type = "service"
       # 更新策略
       update {
         max_parallel = 1
         min_healthy_time = "5m"
         healthy_deadline = "15m"
         progress_deadline = "16m"
         auto_revert = false
         canary = 0
       }

       group "${op_type}" {
         max_client_disconnect = "2h"
         count = ${model_count}
         restart {
           attempts = 1
           interval = "30m"
           delay = "15s"
           mode = "fail"
         }
         ${constraint}
         task "${op_type}" {
           env {
             GRAPHFLOW_OP_TYPE    = "\${NOMAD_GROUP_NAME}"
             MODEL_DIR_ID         = "/home/qspace/model/\${NOMAD_GROUP_NAME}_\${NOMAD_SHORT_ALLOC_ID}"
             ARTIFACT_SERVER_ADDR = "http://localhost:1087"

             MODEL_COS_PATH          = "${model_path}"
              # 模型构件的MD5
             MODEL_MD5               = "${model_md5}"
              # 设置模型的PREFETCH
             GRAPHFLOW_MODEL_N_FETCH = ${prefetch}
              # 设置模型的CONCURRENCY
             GRAPHFLOW_CONCURRENCY   = ${model_concurrency}
              # 一个模型进程对应的logic worker数量
             LOGIC_WORKER_PER_DAEMON = ${samosa_logic_worker_num}
            ${varaStr}
           }
           driver = "raw_exec"
           config {
             command = "/bin/sh"
             args    = [
               "-c",
               "chmod a+x local/start.sh && exec local/start.sh"
             ]
           }

           template {
             data        = <<EOF
      #!/bin/bash
      filedir=$(curl --request POST ''\${ARTIFACT_SERVER_ADDR}'/artifacts?path='\${MODEL_COS_PATH}'&md5='\${MODEL_MD5}'')
      echo \${filedir}
      mkdir -p \${MODEL_DIR_ID}
      cd \${MODEL_DIR_ID} && chmod a+x \${filedir}/main.sh && exec \${filedir}/main.sh
      EOF
             destination = "local/start.sh"
           }
           kill_timeout = "25s"
         }

         task "clean" {
           env {
             MODEL_DIR_ID         = "/home/qspace/model/\${NOMAD_GROUP_NAME}_\${NOMAD_SHORT_ALLOC_ID}"
             ARTIFACT_SERVER_ADDR = "http://localhost:1087"
             MODEL_COS_PATH          = "${model_path}"
             MODEL_MD5               = "${model_md5}"
           }
           lifecycle {
             hook = "poststop"
           }
           driver = "raw_exec"
           config {
             command = "/bin/sh"
             args    = [
               "-c",
                "chmod a+x local/clean.sh && exec local/clean.sh"
             ]
           }

           template {
             data        = <<EOF
      #!/bin/bash
      curl --request DELETE \${ARTIFACT_SERVER_ADDR}'/artifacts?path='\${MODEL_COS_PATH}'&md5='\${MODEL_MD5} -w '\\n'
      curl --request POST \${ARTIFACT_SERVER_ADDR}'/artifacts/_prune' -w '\\n'
      rm -rf \${MODEL_DIR_ID}
      echo clean done
      EOF
             destination = "local/clean.sh"
           }
         }
       }
      }
          `
      );
      return job;
    });
  }

  resetController(controller, isExiting) {
    if (isExiting) {
      controller.model.deleteRecord();
    }
  }
}
