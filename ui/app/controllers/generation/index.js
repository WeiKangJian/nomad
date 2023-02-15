import Controller from '@ember/controller';
import { action } from '@ember/object';
import { inject as service } from '@ember/service';

export default class GenerationController extends Controller {
  @service router;

  @action
  handleClick() {
    let emptyList = ['extra_env', 'deploy_ip'];
    for (let key in this.model) {
      if (!emptyList.includes(key) && this.model[key] === '') {
        alert(key + '不能为空');
        return;
      }
    }
    this.gotoClients();
  }

  @action
  gotoClients() {
    window.localStorage.gVal = JSON.stringify(this.get('model'));
    this.router.transitionTo('jobs.runmodel');
  }
}
