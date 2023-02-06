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
    const { name, type, path, md5, count, prefetch, con, ip, sam, vara } = gVal;

    let varaArr = vara.split(',');
    let varaStr = varaArr.join('\n              ');
    let constraint = '';
    if (ip) {
      constraint = `
        constraint {
          attribute = "\${attr.unique.network.ip-address}"
          operator  = "set_contains_any"
          value     = "${ip}"
        }
      `;
    }
    // When jobs are created with a namespace attribute, it is verified against
    // available namespaces to prevent redirecting to a non-existent namespace.
    return this.store.findAll('namespace').then(() => {
      const job = this.store.createRecord('job');
      job.set(
        '_newDefinition',
        `job "${type}@${name}" {
       datacenters = ["${name}"]
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

       group "${type}" {
         max_client_disconnect = "2h"
         count = ${count}
         restart {
           attempts = 1
           interval = "30m"
           delay = "15s"
           mode = "fail"
         }
         ${constraint}
         task "${type}" {
           env {
             GRAPHFLOW_OP_TYPE    = "\${NOMAD_TASK_NAME}"
             MODEL_DIR_ID         = "/home/qspace/model/\${NOMAD_TASK_NAME}_\${NOMAD_SHORT_ALLOC_ID}"
             ARTIFACT_SERVER_ADDR = "http://localhost:1087"

             MODEL_COS_PATH          = "${path}"
              # 模型构件的MD5
             MODEL_MD5               = "${md5}"
              # 设置模型的PREFETCH
             GRAPHFLOW_MODEL_N_FETCH = ${prefetch}
              # 设置模型的CONCURRENCY
             GRAPHFLOW_CONCURRENCY   = ${con}
              # 一个模型进程对应的logic worker数量
             LOGIC_WORKER_PER_DAEMON = ${sam}
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
